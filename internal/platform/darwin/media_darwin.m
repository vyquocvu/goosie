// media_darwin.m is the macOS playback backend: AVPlayer for the audio and the
// clock, AVPlayerItemVideoOutput for the pixels, and no view at all. Frames come
// out as CVPixelBuffers and go back to Go as premultiplied RGBA, where the
// ordinary tile path composites them like any other image.
//
// This file is manual reference counting, because the package's cgo flags do not
// enable ARC and shim_darwin.m does not either. Everything stored past the scope
// that created it is retained here and released on teardown.
//
// Threading is the part worth reading before changing anything, and it is shaped
// by one measured fact: AVPlayer only works on the process main thread. Give it a
// worker thread with its own CFRunLoop, turn that loop faithfully, and the item's
// status sits at Unknown forever — no KVO notification, no error, rate reports
// 1.0 while the position stays at zero. Pixels, on the other hand, can be taken
// from any thread. So this file splits accordingly:
//
//   - Everything AVFoundation owns — the asset, the item, the player, its
//     observers, and every read of its properties — happens on the main thread
//     through goosieOnMain. The browser already keeps that thread's run loop
//     turning: GoosieAppRun services AppKit with bounded waits, which drains the
//     main queue.
//   - The pixel copy and its channel conversion happen on a per-player GCD
//     queue, because a 1080p frame is too much byte-shuffling to put on the
//     thread that also draws the page.
//
// Two rules from shim.h carry over: no native object ever holds a Go pointer, and
// no call from Go blocks on the decoder except teardown, which has to know the
// audio has actually stopped before it returns.
#import <AVFoundation/AVFoundation.h>
#import <Block.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>
#import <QuartzCore/QuartzCore.h>
#import <math.h>
#import <stdlib.h>
#import <string.h>
#import "media.h"

@class GoosieMediaContext;

// GoosieMediaObserver is the KVO sink for the player item's status. It exists
// because status is the one thing this backend cannot infer: reading it says
// Unknown or Ready but never why, and "why" is the difference between an asset
// this build cannot decode and one that was simply never given a run loop.
@interface GoosieMediaObserver : NSObject {
  @public
	GoosieMediaContext *ctx;
}
@end

// GoosieMediaContext is everything one player owns. It is an object rather than a
// bare struct so a block dispatched to the main queue retains it for as long as
// it is pending, which is what makes teardown safe to call from a thread that is
// not the main one.
@interface GoosieMediaContext : NSObject {
  @public
	AVURLAsset *asset;
	AVPlayerItem *item;
	AVPlayerItemVideoOutput *output;
	AVPlayer *avplayer;
	GoosieMediaObserver *observer;
	id endObserver;
	// timeObserver is the token addPeriodicTimeObserverForInterval: returned.
	// It has to be kept: a discarded return value is autoreleased and can go
	// away while AVPlayer still holds the registration, and it has to be handed
	// back to removeTimeObserver: before the player is released.
	id timeObserver;
	dispatch_queue_t sampleQueue;

	// Observed state. Every read and write holds @synchronized(self), which is
	// also what makes the frame slot safe between the sample queue and a poll.
	int state;
	int errorCode;
	int destroyed;
	int constructed;
	double position;
	double duration;
	int width;
	int height;
	int muted;
	int wantPlay;
	unsigned long long playOp;
	unsigned long long seekOp;
	unsigned long long doneOp;

	// The frame slot: one buffer, replaced wholesale. A frame is copied out
	// exactly once, because the buffer is reused by the next sample and pixels
	// that reached a display list may not change underneath it.
	unsigned char *frame;
	int frameBytes;
	int frameW;
	int frameH;
	int hasFrame;
	unsigned long long frameSeq;
	int sampleOutstanding;
	int wantSample;
	int refreshOutstanding;
	double lastSampleHost;
	double minSampleInterval;

	// Admission limits, read once at creation and applied when the metadata
	// arrives.
	int limitW;
	int limitH;
	int limitPx;
	double limitRate;
	double limitDur;
}
@end

@implementation GoosieMediaObserver
- (void)observeValueForKeyPath:(NSString *)keyPath
                      ofObject:(id)object
                        change:(NSDictionary *)change
                       context:(void *)context {
	@synchronized(ctx) {
		if (ctx->destroyed) {
			return;
		}
		// A failed item is terminal, and its error is the only explanation
		// AVFoundation offers, so it is worth carrying even though the
		// engine reduces it to a code.
		if ([(AVPlayerItem *)object status] == AVPlayerItemStatusFailed) {
			ctx->state = GOOSIE_MEDIA_FAILED;
			ctx->errorCode = GOOSIE_MEDIA_ERR_UNSUPPORTED;
			if (ctx->wantPlay) {
				ctx->wantPlay = 0;
				ctx->doneOp = ctx->playOp;
			}
		}
	}
}
@end

@implementation GoosieMediaContext
- (void)dealloc {
	if (observer) {
		[observer release];
		observer = nil;
	}
	if (endObserver) {
		[[NSNotificationCenter defaultCenter] removeObserver:endObserver];
		[endObserver release];
		endObserver = nil;
	}
	if (timeObserver) {
		[timeObserver release];
		timeObserver = nil;
	}
	if (avplayer) {
		[avplayer release];
		avplayer = nil;
	}
	if (output) {
		[output release];
		output = nil;
	}
	if (item) {
		[item release];
		item = nil;
	}
	if (asset) {
		[asset release];
		asset = nil;
	}
	if (sampleQueue) {
		dispatch_release(sampleQueue);
		sampleQueue = NULL;
	}
	free(frame);
	frame = NULL;
	[super dealloc];
}
@end

// GoosieMediaPlayer is the handle Go holds. It owns one reference to the context;
// the blocks pending on the main and sample queues own the rest.
struct GoosieMediaPlayer {
	GoosieMediaContext *ctx;
};

// goosieOnMain runs block on the process main thread. wait is YES only for
// teardown, where the caller has to know the audio stopped before it returns;
// every command uses NO, because the browser's UI thread must not wait on a
// decoder.
//
// dispatch_sync to the main queue needs that queue to be drained, which is true
// in the browser (GoosieAppRun services it) and in the native tests (their
// TestMain pumps the main run loop). A process with neither would hang here
// rather than fail, so this is a documented requirement of the backend, not a
// possibility it defends against.
static void goosieOnMain(void (^block)(void), BOOL wait) {
	if ([NSThread isMainThread]) {
		block();
		return;
	}
	if (wait) {
		dispatch_sync(dispatch_get_main_queue(), block);
		return;
	}
	dispatch_async(dispatch_get_main_queue(), block);
}

static void goosieFailLocked(GoosieMediaContext *ctx, int code) {
	ctx->state = GOOSIE_MEDIA_FAILED;
	ctx->errorCode = code;
	if (ctx->wantPlay) {
		ctx->wantPlay = 0;
		ctx->doneOp = ctx->playOp;
	}
}

static double goosieSeconds(CMTime t) {
	if (!CMTIME_IS_VALID(t) || CMTIME_IS_INDEFINITE(t)) {
		return -1;
	}
	double s = CMTimeGetSeconds(t);
	return isnan(s) ? -1 : s;
}

#pragma mark - Sampling (any thread)

// goosieSample takes the frame at the player's current time and converts it into
// the context's slot. Runs on the sample queue: reading pixels is the one part of
// AVFoundation that was measured to work away from the main thread, and it is
// also the part too expensive to put there.
static void goosieSample(GoosieMediaContext *ctx) {
	CMTime t;
	@synchronized(ctx) {
		if (ctx->destroyed || !ctx->output) {
			ctx->sampleOutstanding = 0;
			return;
		}
		ctx->sampleOutstanding = 1;
		ctx->wantSample = 0;
		// The output's own host-time mapping is asked for the time whether the
		// player is running or not. It is frozen while paused and moves on a
		// seek, which makes it the one time the output is guaranteed to be able
		// to serve: reading the item's currentTime instead asks for an exact
		// instant that can fall between two frames, and the output answers that
		// with no buffer at all.
		t = [ctx->output itemTimeForHostTime:CACurrentMediaTime()];
	}
	if (!CMTIME_IS_VALID(t)) {
		@synchronized(ctx) {
			ctx->sampleOutstanding = 0;
			ctx->wantSample = 1;
		}
		return;
	}
	CVPixelBufferRef pb = [ctx->output copyPixelBufferForItemTime:t itemTimeForDisplay:NULL];
	if (!pb) {
		@synchronized(ctx) {
			ctx->sampleOutstanding = 0;
			// Nothing was available at that instant. Ask again rather
			// than leaving a paused video permanently blank because its
			// first attempt came too early.
			ctx->wantSample = 1;
		}
		return;
	}

	@autoreleasepool {
		if (CVPixelBufferGetPixelFormatType(pb) == kCVPixelFormatType_32BGRA &&
		    CVPixelBufferLockBaseAddress(pb, kCVPixelBufferLock_ReadOnly) == kCVReturnSuccess) {
			size_t w = CVPixelBufferGetWidth(pb);
			size_t h = CVPixelBufferGetHeight(pb);
			size_t stride = CVPixelBufferGetBytesPerRow(pb);
			const unsigned char *src = (const unsigned char *)CVPixelBufferGetBaseAddress(pb);
			if (src && w > 0 && h > 0 && stride >= w * 4 && w < 16384 && h < 16384) {
				int bytes = (int)(w * h * 4);
				unsigned char *dst = NULL;
				@synchronized(ctx) {
					if (ctx->frameBytes < bytes) {
						free(ctx->frame);
						ctx->frame = (unsigned char *)malloc((size_t)bytes);
						ctx->frameBytes = ctx->frame ? bytes : 0;
					}
					dst = ctx->frame;
				}
				if (dst) {
					// BGRA premultiplied becomes RGBA premultiplied
					// by swapping the outer two channels; the alpha
					// needs no arithmetic because both formats are
					// already premultiplied.
					for (size_t y = 0; y < h; y++) {
						const unsigned char *srow = src + y * stride;
						unsigned char *drow = dst + y * w * 4;
						for (size_t x = 0; x < w; x++) {
							drow[x * 4 + 0] = srow[x * 4 + 2];
							drow[x * 4 + 1] = srow[x * 4 + 1];
							drow[x * 4 + 2] = srow[x * 4 + 0];
							drow[x * 4 + 3] = srow[x * 4 + 3];
						}
					}
					@synchronized(ctx) {
						if (!ctx->destroyed) {
							ctx->frameW = (int)w;
							ctx->frameH = (int)h;
							ctx->hasFrame = 1;
							ctx->frameSeq++;
							ctx->position = goosieSeconds(t);
						}
					}
				}
			}
			CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
		}
	}
	CVPixelBufferRelease(pb);
	@synchronized(ctx) {
		ctx->sampleOutstanding = 0;
		ctx->lastSampleHost = CACurrentMediaTime();
	}
}

// goosieRequestSample posts a sample if one is due. Called from a poll, so it
// must not block: the request is queued and the pixels arrive later.
static void goosieRequestSample(GoosieMediaContext *ctx) {
	int should = 0;
	@synchronized(ctx) {
		if (ctx->destroyed || ctx->sampleOutstanding || ctx->state == GOOSIE_MEDIA_FAILED ||
		    !ctx->sampleQueue) {
			return;
		}
		if (ctx->state == GOOSIE_MEDIA_PLAYING) {
			if (CACurrentMediaTime() - ctx->lastSampleHost >= ctx->minSampleInterval) {
				should = 1;
			}
		} else if (ctx->wantSample || ctx->wantPlay) {
			// wantPlay keeps asking until the play settles, which is how a
			// request made before the item was ready still gets answered.
			ctx->wantSample = 0;
			should = 1;
		}
	}
	if (should) {
		dispatch_async(ctx->sampleQueue, ^{
			goosieSample(ctx);
		});
	}
}

#pragma mark - Main-thread work

// goosieRefresh caches everything Go is allowed to read into the context. It runs
// on the main thread because every property it touches belongs to AVFoundation,
// and it is what lets a poll answer without going near the framework.
static void goosieRefresh(GoosieMediaContext *ctx) {
	@synchronized(ctx) {
		ctx->refreshOutstanding = 0;
		if (ctx->destroyed || !ctx->item || !ctx->avplayer) {
			return;
		}
		double pos = goosieSeconds([ctx->avplayer currentTime]);
		if (pos >= 0) {
			ctx->position = pos;
		}
		int ready = ([ctx->item status] == AVPlayerItemStatusReadyToPlay);
		if (ctx->wantPlay && ready && [ctx->avplayer rate] != 0 &&
		    ctx->state != GOOSIE_MEDIA_FAILED) {
			ctx->wantPlay = 0;
			ctx->state = GOOSIE_MEDIA_PLAYING;
			ctx->doneOp = ctx->playOp;
			ctx->wantSample = 1;
		} else if (ctx->state == GOOSIE_MEDIA_PLAYING && [ctx->avplayer rate] == 0) {
			ctx->state = GOOSIE_MEDIA_PAUSED;
		}
	}
}

// goosieRequestRefresh asks for one refresh, coalesced: a poll happens far more
// often than the main thread needs to be bothered, and more than one pending
// refresh would only read the same properties twice.
static void goosieRequestRefresh(GoosieMediaContext *ctx) {
	int should = 0;
	@synchronized(ctx) {
		if (ctx->destroyed || ctx->refreshOutstanding) {
			return;
		}
		ctx->refreshOutstanding = 1;
		should = 1;
	}
	if (should) {
		goosieOnMain(^{
			goosieRefresh(ctx);
		}, NO);
	}
}

// goosieInspect decides whether this backend is willing to play the asset and
// publishes its metadata. Runs on the main thread after the keys below have been
// loaded, so statusOfValueForKey: answers Loaded rather than Unknown — it reports
// whether a load already finished and does not start one.
static void goosieInspect(GoosieMediaContext *ctx) {
	NSError *err = nil;
	AVKeyValueStatus playable = [ctx->asset statusOfValueForKey:@"playable" error:&err];
	AVKeyValueStatus trackStatus = [ctx->asset statusOfValueForKey:@"tracks" error:&err];
	AVKeyValueStatus durStatus = [ctx->asset statusOfValueForKey:@"duration" error:&err];

	@synchronized(ctx) {
		if (ctx->destroyed) {
			return;
		}
		if (playable != AVKeyValueStatusLoaded || trackStatus != AVKeyValueStatusLoaded ||
		    durStatus != AVKeyValueStatusLoaded || err != nil) {
			// err being set here is the interesting case: with ForbidAll it
			// means the file tried to reference data outside itself, which
			// is exactly what the downloaded-copy design exists to prevent.
			goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED);
			return;
		}
		if (![ctx->asset isPlayable]) {
			goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED);
			return;
		}

		double dur = goosieSeconds(ctx->asset.duration);
		if (dur <= 0 || (ctx->limitDur > 0 && dur > ctx->limitDur)) {
			goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED);
			return;
		}

		NSArray *tracks = [ctx->asset tracksWithMediaType:AVMediaTypeVideo];
		if ([tracks count] == 0) {
			// Audio-only is not this element's job, and a video box with
			// nothing to draw would sit there blank forever.
			goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED);
			return;
		}
		AVAssetTrack *track = [tracks objectAtIndex:0];

		CGAffineTransform tf = [track preferredTransform];
		if (tf.b != 0 || tf.c != 0) {
			// A rotation or a shear would need the pixels turned before
			// they are handed over. This backend does not do that yet, and
			// reporting a size the pixels do not have would put a sideways
			// video on screen while claiming it was fine.
			goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED);
			return;
		}
		CGSize nat = [track naturalSize];
		CGSize disp = CGSizeApplyAffineTransform(nat, tf);
		int w = (int)fabs(disp.width);
		int h = (int)fabs(disp.height);
		if (w <= 0 || h <= 0) {
			goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED);
			return;
		}
		if ((ctx->limitW > 0 && w > ctx->limitW) || (ctx->limitH > 0 && h > ctx->limitH) ||
		    (ctx->limitPx > 0 && (long long)w * (long long)h > (long long)ctx->limitPx)) {
			goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED);
			return;
		}
		float rate = [track nominalFrameRate];
		if (rate > 0 && ctx->limitRate > 0 && (double)rate > ctx->limitRate * 1.5) {
			// Half again the cap is the refusal point rather than the cap
			// itself: a 30fps asset is routinely stored at 30000/1001, and
			// refusing that would refuse ordinary video. Frames above the
			// cap are dropped by the sampler, not by this check.
			goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED);
			return;
		}

		ctx->duration = dur;
		ctx->width = w;
		ctx->height = h;
		ctx->state = GOOSIE_MEDIA_READY;
		// One frame at the head, so a paused video shows something before the
		// user asks it to play.
		ctx->wantSample = 1;
	}
}

// goosieConstruct builds the AVFoundation objects. Runs on the main thread, which
// is not a style choice: an item created anywhere else never becomes ready.
static void goosieConstruct(GoosieMediaContext *ctx, NSString *nsPath) {
	@autoreleasepool {
		NSURL *url = [NSURL fileURLWithPath:nsPath];
		// ForbidAll is the reason a downloaded file can be handed to a native
		// stack at all: an ISO base media file may point at data in another
		// file or on another host, and this refuses to follow any of it. The
		// asset is the bytes the browser fetched and nothing else.
		NSDictionary *opts = [NSDictionary dictionaryWithObjectsAndKeys:
		                                            [NSNumber numberWithUnsignedInteger:AVAssetReferenceRestrictionForbidAll],
		                                        AVURLAssetReferenceRestrictionsKey,
		                                            [NSNumber numberWithBool:YES],
		                                        AVURLAssetPreferPreciseDurationAndTimingKey, nil];
		ctx->asset = [[AVURLAsset alloc] initWithURL:url options:opts];
		if (!ctx->asset) {
			@synchronized(ctx) { goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED); }
			return;
		}
		ctx->item = [[AVPlayerItem alloc] initWithAsset:ctx->asset];
		if (!ctx->item) {
			@synchronized(ctx) { goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED); }
			return;
		}

		NSDictionary *pixAttrs = [NSDictionary dictionaryWithObject:[NSNumber numberWithUnsignedInt:kCVPixelFormatType_32BGRA]
		                                                             forKey:(id)kCVPixelBufferPixelFormatTypeKey];
		ctx->output = [[AVPlayerItemVideoOutput alloc] initWithPixelBufferAttributes:pixAttrs];
		if (!ctx->output) {
			@synchronized(ctx) { goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED); }
			return;
		}
		// The pixels are ours to composite. Leaving this at NO would have
		// AVPlayer render them a second time through a layer nobody asked for.
		[ctx->output setSuppressesPlayerRendering:YES];
		[ctx->item addOutput:ctx->output];

		ctx->avplayer = [[AVPlayer alloc] initWithPlayerItem:ctx->item];
		if (!ctx->avplayer) {
			@synchronized(ctx) { goosieFailLocked(ctx, GOOSIE_MEDIA_ERR_UNSUPPORTED); }
			return;
		}
		// Pause at the end rather than rewind: the engine reports ended and a
		// replay is a seek to zero, which is what the DOM says happens too.
		[ctx->avplayer setActionAtItemEnd:AVPlayerActionAtItemEndPause];

		ctx->observer = [[GoosieMediaObserver alloc] init];
		ctx->observer->ctx = ctx;
		[ctx->item addObserver:ctx->observer
		            forKeyPath:@"status"
		               options:NSKeyValueObservingOptionNew
		               context:NULL];

		// Retained, like the time observer's token: an unretained return value
		// from a notification registration can be released while AVFoundation
		// still holds it, leaving teardown to call removeObserver: on freed
		// memory.
		ctx->endObserver = [[[NSNotificationCenter defaultCenter] addObserverForName:AVPlayerItemDidPlayToEndTimeNotification
		                                                                     object:ctx->item
		                                                                      queue:nil
		                                                                 usingBlock:^(NSNotification *note) {
			                  @synchronized(ctx) {
				                  if (ctx->destroyed) {
					                  return;
				                  }
				                  ctx->state = GOOSIE_MEDIA_ENDED;
				                  if (ctx->wantPlay) {
					                  ctx->wantPlay = 0;
					                  ctx->doneOp = ctx->playOp;
				                  }
				                  ctx->wantSample = 1;
			                  }
		                  }] retain];

		// The position observer is delivered on the sample queue rather than the
		// main one: it fires several times a second and only writes a cached
		// double, so there is no reason to spend main-thread time on it.
		CMTime interval = CMTimeMakeWithSeconds(ctx->minSampleInterval, NSEC_PER_SEC);
		ctx->timeObserver = [[ctx->avplayer addPeriodicTimeObserverForInterval:interval
		                                                                 queue:ctx->sampleQueue
		                                                            usingBlock:^(CMTime t) {
		                         double s = goosieSeconds(t);
		                         @synchronized(ctx) {
			                         if (ctx->destroyed || s < 0) {
				                         return;
			                         }
			                         ctx->position = s;
		                         }
	                         }] retain];

		@synchronized(ctx) { ctx->constructed = 1; }

		NSArray *keys = [NSArray arrayWithObjects:@"tracks", @"duration", @"playable", nil];
		[ctx->asset loadValuesAsynchronouslyForKeys:keys completionHandler:^{
			goosieOnMain(^{
				goosieInspect(ctx);
			}, NO);
		}];
	}
}

#pragma mark - C entry points

GoosieMediaPlayer *GoosieMediaPlayerCreate(const char *path,
                                           int maxWidth, int maxHeight,
                                           int maxPixels,
                                           double maxFrameRate, double maxDuration,
                                           int *err) {
	if (err) {
		*err = GOOSIE_MEDIA_CREATE_OK;
	}
	if (!path || !*path) {
		if (err) {
			*err = GOOSIE_MEDIA_CREATE_BAD_ARG;
		}
		return NULL;
	}
	// alloc/initWithUTF8String rather than stringWithUTF8String: the caller is a
	// Go thread with no autorelease pool, so an autoreleased string here would be
	// registered with a pool that never drains. The block below retains it, so
	// dropping this reference is enough.
	NSString *nsPath = [[NSString alloc] initWithUTF8String:path];
	if (!nsPath) {
		if (err) {
			*err = GOOSIE_MEDIA_CREATE_BAD_ARG;
		}
		return NULL;
	}
	GoosieMediaPlayer *p = (GoosieMediaPlayer *)calloc(1, sizeof(GoosieMediaPlayer));
	if (!p) {
		if (err) {
			*err = GOOSIE_MEDIA_CREATE_FAILED;
		}
		return NULL;
	}
	GoosieMediaContext *ctx = [[GoosieMediaContext alloc] init];
	ctx->state = GOOSIE_MEDIA_PREPARING;
	ctx->duration = -1;
	ctx->limitW = maxWidth;
	ctx->limitH = maxHeight;
	ctx->limitPx = maxPixels;
	ctx->limitRate = maxFrameRate;
	ctx->limitDur = maxDuration;
	ctx->minSampleInterval = (maxFrameRate > 0) ? (1.0 / maxFrameRate) : (1.0 / 30.0);
	ctx->sampleQueue = dispatch_queue_create("dev.goosie.media.sample", DISPATCH_QUEUE_SERIAL);
	p->ctx = ctx;

	// Construction is posted rather than performed: whether the file is playable
	// is an answer that arrives through the state, so there is nothing for the
	// caller to wait on and no reason to put a main-thread round trip in the
	// middle of opening a video.
	goosieOnMain(^{
		goosieConstruct(ctx, nsPath);
	}, NO);
	[nsPath release];
	return p;
}

void GoosieMediaPlayerPlay(GoosieMediaPlayer *p, unsigned long long op) {
	if (!p || !p->ctx) {
		return;
	}
	GoosieMediaContext *ctx = p->ctx;
	goosieOnMain(^{
		@synchronized(ctx) {
			if (ctx->destroyed || ctx->state == GOOSIE_MEDIA_FAILED || !ctx->avplayer) {
				return;
			}
			ctx->wantPlay = 1;
			ctx->playOp = op;
			ctx->doneOp = 0;
			ctx->wantSample = 1;
		}
		[ctx->avplayer play];
		goosieRefresh(ctx);
	}, NO);
}

void GoosieMediaPlayerPause(GoosieMediaPlayer *p) {
	if (!p || !p->ctx) {
		return;
	}
	GoosieMediaContext *ctx = p->ctx;
	goosieOnMain(^{
		@synchronized(ctx) {
			if (ctx->destroyed || !ctx->avplayer) {
				return;
			}
			ctx->wantPlay = 0;
		}
		[ctx->avplayer pause];
		@synchronized(ctx) {
			if (ctx->state == GOOSIE_MEDIA_PLAYING || ctx->state == GOOSIE_MEDIA_READY ||
			    ctx->state == GOOSIE_MEDIA_ENDED) {
				ctx->state = GOOSIE_MEDIA_PAUSED;
			}
			ctx->wantSample = 1;
		}
	}, NO);
}

void GoosieMediaPlayerSeek(GoosieMediaPlayer *p, unsigned long long op, double seconds) {
	if (!p || !p->ctx || !(seconds >= 0)) {
		return;
	}
	GoosieMediaContext *ctx = p->ctx;
	goosieOnMain(^{
		@synchronized(ctx) {
			if (ctx->destroyed || !ctx->avplayer) {
				return;
			}
			ctx->seekOp = op;
			ctx->doneOp = 0;
		}
		CMTime t = CMTimeMakeWithSeconds(seconds, NSEC_PER_SEC);
		[ctx->avplayer seekToTime:t
		            toleranceBefore:kCMTimeZero
		             toleranceAfter:kCMTimeZero
		          completionHandler:^(BOOL finished) {
			            goosieOnMain(^{
				            @synchronized(ctx) {
					            if (ctx->destroyed) {
						            return;
					            }
					            // A seek another seek superseded reports against
					            // the newest op only. The engine drops the older
					            // one, and reporting it here would let a paused
					            // video settle twice.
					            if (finished && ctx->seekOp == op) {
						            ctx->doneOp = op;
						            ctx->wantSample = 1;
					            }
					            double s = goosieSeconds([ctx->item currentTime]);
					            if (s >= 0) {
						            ctx->position = s;
					            }
					            if ([ctx->avplayer rate] != 0 && ctx->state != GOOSIE_MEDIA_FAILED) {
						            ctx->state = GOOSIE_MEDIA_PLAYING;
					            } else if (ctx->state == GOOSIE_MEDIA_PLAYING) {
						            ctx->state = GOOSIE_MEDIA_PAUSED;
					            }
				            }
			            }, NO);
		            }];
	}, NO);
}

void GoosieMediaPlayerSetMuted(GoosieMediaPlayer *p, int muted) {
	if (!p || !p->ctx) {
		return;
	}
	GoosieMediaContext *ctx = p->ctx;
	goosieOnMain(^{
		@synchronized(ctx) {
			if (ctx->destroyed || !ctx->avplayer) {
				return;
			}
			ctx->muted = muted ? 1 : 0;
		}
		[ctx->avplayer setMuted:(muted ? YES : NO)];
	}, NO);
}

void GoosieMediaPlayerPoll(GoosieMediaPlayer *p, GoosieMediaState *out) {
	if (!p || !p->ctx || !out) {
		return;
	}
	GoosieMediaContext *ctx = p->ctx;
	memset(out, 0, sizeof(*out));
	out->duration = -1;
	// Both requests are coalesced and neither waits: a poll is on the browser's
	// UI thread and its whole job is to report what is already known.
	goosieRequestRefresh(ctx);
	goosieRequestSample(ctx);
	@synchronized(ctx) {
		out->state = ctx->state;
		out->errorCode = ctx->errorCode;
		out->position = ctx->position < 0 ? 0 : ctx->position;
		out->duration = ctx->duration;
		out->width = ctx->width;
		out->height = ctx->height;
		out->muted = ctx->muted;
		out->doneOp = ctx->doneOp;
		out->frameSeq = ctx->frameSeq;
		out->hasFrame = ctx->hasFrame;
		out->frameW = ctx->frameW;
		out->frameH = ctx->frameH;
		out->frameBytes = ctx->hasFrame ? (ctx->frameW * ctx->frameH * 4) : 0;
		// Delivered once, so two polls of the same state cannot settle the same
		// operation twice.
		ctx->doneOp = 0;
	}
}

int GoosieMediaPlayerFrame(GoosieMediaPlayer *p, unsigned char *buf, int cap) {
	if (!p || !p->ctx || !buf || cap <= 0) {
		return 0;
	}
	GoosieMediaContext *ctx = p->ctx;
	@synchronized(ctx) {
		if (!ctx->hasFrame || !ctx->frame) {
			return 0;
		}
		int bytes = ctx->frameW * ctx->frameH * 4;
		if (bytes <= 0 || bytes > cap) {
			return 0;
		}
		memcpy(buf, ctx->frame, (size_t)bytes);
		ctx->hasFrame = 0;
		return bytes;
	}
}

void GoosieMediaPlayerDestroy(GoosieMediaPlayer *p) {
	if (!p) {
		return;
	}
	GoosieMediaContext *ctx = p->ctx;
	p->ctx = NULL;
	if (!ctx) {
		free(p);
		return;
	}
	@synchronized(ctx) {
		ctx->destroyed = 1;
		ctx->wantPlay = 0;
	}
	// Audio stops first, and on the main thread, because the rest of this
	// function is bookkeeping: teardown that returns while a video is still
	// audible is the one failure a user would notice.
	goosieOnMain(^{
		@synchronized(ctx) {
			[ctx->avplayer pause];
			if (ctx->observer) {
				[ctx->item removeObserver:ctx->observer forKeyPath:@"status"];
				[ctx->observer release];
				ctx->observer = nil;
			}
			if (ctx->timeObserver) {
				[ctx->avplayer removeTimeObserver:ctx->timeObserver];
				[ctx->timeObserver release];
				ctx->timeObserver = nil;
			}
			if (ctx->endObserver) {
				[[NSNotificationCenter defaultCenter] removeObserver:ctx->endObserver];
				[ctx->endObserver release];
				ctx->endObserver = nil;
			}
			if (ctx->avplayer) {
				[ctx->avplayer release];
				ctx->avplayer = nil;
			}
			if (ctx->output) {
				[ctx->output release];
				ctx->output = nil;
			}
			if (ctx->item) {
				[ctx->item release];
				ctx->item = nil;
			}
			if (ctx->asset) {
				[ctx->asset release];
				ctx->asset = nil;
			}
		}
	}, YES);
	// Draining the sample queue means no pixel copy is mid-flight when the last
	// reference goes. The block checks destroyed and returns, so this waits on
	// one copy at most.
	if (ctx->sampleQueue) {
		dispatch_sync(ctx->sampleQueue, ^{
		});
	}
	[ctx release];
	free(p);
}
