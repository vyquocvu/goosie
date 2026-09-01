//go:build darwin && cgo

package ui

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreVideo
#include <CoreGraphics/CoreGraphics.h>
#include <CoreVideo/CoreVideo.h>

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

static double getDisplayRefreshRate() {
	CGDirectDisplayID displayID = CGMainDisplayID();
	CGDisplayModeRef mode = CGDisplayCopyDisplayMode(displayID);
	double refreshRate = 0.0;
	if (mode) {
		refreshRate = CGDisplayModeGetRefreshRate(mode);
		CGDisplayModeRelease(mode);
	}
	if (refreshRate <= 0.0) {
		CVDisplayLinkRef link = NULL;
		if (CVDisplayLinkCreateWithCGDisplay(displayID, &link) == kCVReturnSuccess && link != NULL) {
			CVTime time = CVDisplayLinkGetNominalOutputVideoRefreshPeriod(link);
			if (time.timeScale > 0 && time.timeValue > 0) {
				refreshRate = (double)time.timeScale / (double)time.timeValue;
			}
			CVDisplayLinkRelease(link);
		}
	}
	if (refreshRate <= 0.0) {
		refreshRate = 60.0;
	}
	return refreshRate;
}
#pragma clang diagnostic pop
*/
import "C"
import (
	"os"
	"strconv"
)

// GetDeviceScreenRefreshRate returns the refresh rate of the primary display in Hz.
// If GOOSIE_TARGET_FPS or GOOSIE_FPS environment variable is set, it takes precedence.
func GetDeviceScreenRefreshRate() float64 {
	if env := os.Getenv("GOOSIE_TARGET_FPS"); env != "" {
		if fps, err := strconv.ParseFloat(env, 64); err == nil && fps > 0 {
			return fps
		}
	}
	if env := os.Getenv("GOOSIE_FPS"); env != "" {
		if fps, err := strconv.ParseFloat(env, 64); err == nil && fps > 0 {
			return fps
		}
	}
	return float64(C.getDisplayRefreshRate())
}
