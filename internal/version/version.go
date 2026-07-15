package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

func String() string {
	return fmt.Sprintf("%s (commit %s, built %s, %s)", Version, Commit, BuildTime, runtime.Version())
}

func ReadBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return String()
	}
	commit := Commit
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && Commit == "none" {
			commit = s.Value
		}
	}
	return fmt.Sprintf("%s (commit %s, built %s, %s)", Version, commit, BuildTime, info.GoVersion)
}
