// media.h is the C half of the macOS playback backend. Objective-C lives in
// media_darwin.m and never crosses this boundary, for the same reason the window
// shim is split the same way: the rest of v2 stays pure Go and buildable on a
// machine that has no AVFoundation at all.
//
// Two rules shape the whole header, both inherited from shim.h:
//
//   - No Go pointer is ever stored in C memory. State is copied out by value and
//     pixels are copied into a buffer the caller owns for the length of the call.
//   - No call here blocks on the decoder. Commands are posted to the player's own
//     queue and the newest observation is read back, so a stalled asset slows one
//     video down instead of taking the browser's UI thread with it.
//
// The player deliberately has no view. suppressesPlayerRendering is set on the
// video output, which means AVPlayer keeps the audio and the clock while the
// pixels come to us to composite through the ordinary tile path.
#ifndef GOOSIE_MEDIA_H
#define GOOSIE_MEDIA_H

#ifdef __cplusplus
extern "C" {
#endif

// GoosieMediaPlayer is one asset: an AVPlayer, its video output, and the queue
// every observation and command is serialized on.
typedef struct GoosieMediaPlayer GoosieMediaPlayer;

// State kinds, mapped to media.State in Go. The values are this header's own
// rather than a Go iota's, so reordering one side cannot silently misreport the
// other.
enum {
	GOOSIE_MEDIA_PREPARING = 0,
	GOOSIE_MEDIA_READY = 1,
	GOOSIE_MEDIA_PLAYING = 2,
	GOOSIE_MEDIA_PAUSED = 3,
	GOOSIE_MEDIA_ENDED = 4,
	GOOSIE_MEDIA_FAILED = 5
};

// Failure reasons, mapped to media.ErrorCode in Go.
enum {
	GOOSIE_MEDIA_ERR_NONE = 0,
	GOOSIE_MEDIA_ERR_UNSUPPORTED = 1,
	GOOSIE_MEDIA_ERR_DECODE = 2
};

// Results of GoosieMediaPlayerCreate.
enum {
	GOOSIE_MEDIA_CREATE_OK = 0,
	GOOSIE_MEDIA_CREATE_BAD_ARG = 1,
	GOOSIE_MEDIA_CREATE_FAILED = 2
};

// GoosieMediaState is one observation of a player, copied into Go by value.
//
// duration is negative until the asset's metadata is known, which Go turns into
// the NaN HTMLMediaElement.duration reports at that point: a negative number
// survives the crossing, a NaN in a C double is a value Go would have to
// reinterpret rather than read.
typedef struct {
	int state;
	int errorCode;
	double position;
	double duration;
	int width;
	int height;
	int muted;
	// doneOp is the operation that completed since the last poll, or 0. It is
	// delivered once: reading the state clears it.
	unsigned long long doneOp;
	// frameSeq counts the frames this player has produced, and hasFrame says
	// whether the newest one is still waiting to be copied out.
	unsigned long long frameSeq;
	int hasFrame;
	// frameW, frameH and frameBytes describe the pending frame, so the caller
	// can size its buffer before asking for the pixels.
	int frameW;
	int frameH;
	int frameBytes;
} GoosieMediaState;

// GoosieMediaPlayerCreate begins preparing the file at path. It returns as soon
// as the asset object exists; whether the file is playable arrives through the
// state. The limits bound what the asset may ask for once its metadata is read.
GoosieMediaPlayer *GoosieMediaPlayerCreate(const char *path,
                                           int maxWidth, int maxHeight,
                                           int maxPixels,
                                           double maxFrameRate, double maxDuration,
                                           int *err);

// GoosieMediaPlayerPlay posts a play and reports the outcome against op.
void GoosieMediaPlayerPlay(GoosieMediaPlayer *p, unsigned long long op);

// GoosieMediaPlayerPause posts a pause. It carries no op: there is nothing to
// settle, and a pause that arrives while nothing is playing is not an error.
void GoosieMediaPlayerPause(GoosieMediaPlayer *p);

// GoosieMediaPlayerSeek posts a move to seconds and reports against op.
void GoosieMediaPlayerSeek(GoosieMediaPlayer *p, unsigned long long op, double seconds);

// GoosieMediaPlayerSetMuted silences or restores the audio track.
void GoosieMediaPlayerSetMuted(GoosieMediaPlayer *p, int muted);

// GoosieMediaPlayerPoll copies the newest observation into out and asks the
// player for another frame if one is due. It never waits for either.
void GoosieMediaPlayerPoll(GoosieMediaPlayer *p, GoosieMediaState *out);

// GoosieMediaPlayerFrame copies the pending frame into buf as premultiplied RGBA
// and returns the byte count written, or 0 when there is no pending frame or cap
// is too small. Copying clears the pending flag, so a frame is handed to exactly
// one reader.
int GoosieMediaPlayerFrame(GoosieMediaPlayer *p, unsigned char *buf, int cap);

// GoosieMediaPlayerDestroy stops playback, releases the asset and drops the
// queue. It is idempotent and safe from any thread, and after it returns the
// pointer must not be used again.
void GoosieMediaPlayerDestroy(GoosieMediaPlayer *p);

#ifdef __cplusplus
}
#endif

#endif  // GOOSIE_MEDIA_H
