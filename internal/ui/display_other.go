//go:build !darwin || !cgo

package ui

import (
	"os"
	"strconv"
)

// GetDeviceScreenRefreshRate returns the default refresh rate for platforms where
// native query is unavailable (defaults to 60.0 Hz unless overridden by GOOSIE_TARGET_FPS).
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
	return 60.0
}
