package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/platform"
)

type config struct {
	url                 string
	gate                bool
	bench               bool
	screenshot          bool
	screenshotSeq       string
	seqCount            int
	seqInterval         time.Duration
	seqScrollScreens    int
	printPDF            string
	backend             string
	frames              int
	scene               string
	sceneSet            bool
	out                 string
	width               int
	height              int
	dpr                 float64
	downloadDir         string
	downloadDirExplicit bool
	private             bool
	profile             string
	clearData           bool
	sessionFile         string
	sessionActiveFile   string
	cookieFile          string
	bookmarkFile        string
	historyFile         string
	theme               string
	userscriptDir       string
	appURL              string
	flagsFile           string
	permsFile           string
	preferencesFile     string
}

func (c config) stateDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".goosie"
	}
	base := filepath.Join(home, ".goosie")
	if c.profile == "" {
		return base
	}
	return filepath.Join(base, "profiles", c.profile)
}

func effectiveDownloadDir(cliDir string, explicit bool, profileDir string) (string, error) {
	dir := cliDir
	if !explicit && profileDir != "" {
		dir = profileDir
	}
	if dir != "" && !filepath.IsAbs(dir) {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		dir = absolute
	}
	return dir, nil
}

func (c config) devSize() frame.Size {
	return frame.Size{
		W: int32(float64(c.width)*c.dpr + 0.5),
		H: int32(float64(c.height)*c.dpr + 0.5),
	}
}

func (c config) vsyncPeriod() time.Duration {
	if c.bench {
		return benchPeriod
	}
	return displayPeriod
}

func (c *config) paced() bool {
	return c.gate || c.bench || c.screenshot || c.screenshotSeq != ""
}

func parse(args []string) (config, error) {
	var c config
	fs := flag.NewFlagSet("goosie", flag.ContinueOnError)
	fs.StringVar(&c.url, "url", "", "document to load")
	fs.BoolVar(&c.gate, "gate", false, "drive -frames scripted scroll frames at real pacing, then report")
	fs.BoolVar(&c.bench, "bench", false, "run frames as fast as the clock allows, headless, and report the rate")
	fs.BoolVar(&c.screenshot, "screenshot", false, "render -url to a PNG at -out and exit")
	fs.StringVar(&c.screenshotSeq, "screenshot-seq", "", "write a numbered PNG sequence of one -url session to a directory and exit")
	fs.IntVar(&c.seqCount, "seq-frames", 5, "captures to write with -screenshot-seq")
	fs.DurationVar(&c.seqInterval, "seq-interval", time.Second, "dwell between -screenshot-seq captures")
	fs.IntVar(&c.seqScrollScreens, "seq-scroll-screens", 0, "screens to scroll with -screenshot-seq before the first capture")
	fs.StringVar(&c.printPDF, "print-pdf", "", "export document to PDF at path and exit")
	fs.StringVar(&c.backend, "backend", "", "window backend: headless or native (empty means whatever this machine has)")
	fs.IntVar(&c.frames, "frames", defaultFrames, "frames to draw with -gate and -bench")
	fs.StringVar(&c.scene, "scene", "checkerboard", "synthetic test document: checkerboard or plain")
	fs.StringVar(&c.out, "out", "", "write the frame report as JSON here, or - for stdout")
	fs.IntVar(&c.width, "width", 1440, "viewport width in CSS pixels")
	fs.IntVar(&c.height, "height", 900, "viewport height in CSS pixels")
	fs.Float64Var(&c.dpr, "dpr", 2, "device pixel ratio")
	dlDir, err := os.UserHomeDir()
	if err != nil || dlDir == "" {
		dlDir = "."
	} else {
		dlDir = filepath.Join(dlDir, "Downloads")
	}
	fs.StringVar(&c.downloadDir, "download-dir", dlDir, "directory where downloads are saved")
	fs.BoolVar(&c.private, "private", false, "run a private session: cookies are dropped on exit")
	fs.StringVar(&c.profile, "profile", "", "named profile: state lives under ~/.goosie/profiles/<name> instead of ~/.goosie")
	fs.StringVar(&c.sessionFile, "session-file", "", "file where the open tabs are saved between runs")
	fs.StringVar(&c.theme, "theme", "system", "appearance theme: system, light, dark, high-contrast")
	fs.StringVar(&c.appURL, "app", "", "launch as a standalone web application (PWA mode)")
	fs.BoolVar(&c.clearData, "clear-data", false, "clear browsing data (cookies, history, session; keeps bookmarks) and exit")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: goosie [-scene checkerboard] [-width 1440] [-height 900] [-dpr 2]")
		fmt.Fprintln(fs.Output(), "       goosie -app <url>  (launch standalone web application)")
		fmt.Fprintln(fs.Output(), "       goosie -gate -frames 600 -out gate.json")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return c, fmt.Errorf("%w: help requested", errUsage)
		}
		return c, fmt.Errorf("%w: %v", errUsage, err)
	}
	switch {
	case c.width <= 0 || c.height <= 0:
		return c, fmt.Errorf("%w: -width and -height must be positive", errUsage)
	case math.IsNaN(c.dpr) || c.dpr <= 0 || c.dpr > 8:
		return c, fmt.Errorf("%w: -dpr must be in (0, 8]", errUsage)
	case c.frames <= 0:
		return c, fmt.Errorf("%w: -frames must be positive", errUsage)
	case c.profile == "." || c.profile == ".." || strings.ContainsAny(c.profile, "/\\"):
		return c, fmt.Errorf("%w: -profile must be a bare name", errUsage)
	}
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "scene" {
			c.sceneSet = true
		}
		if fl.Name == "download-dir" {
			c.downloadDirExplicit = true
		}
	})
	if _, err := sceneSpec(c.scene); err != nil {
		return c, err
	}
	if c.bench || c.screenshot || c.screenshotSeq != "" {
		if fs.Lookup("backend").Value.String() == "" {
			c.backend = platform.Headless
		}
	}
	if c.screenshotSeq != "" {
		if c.url == "" {
			return c, fmt.Errorf("%w: -screenshot-seq requires -url", errUsage)
		}
		if c.seqCount < 1 {
			return c, fmt.Errorf("%w: -seq-frames must be at least 1", errUsage)
		}
		if c.seqInterval <= 0 {
			return c, fmt.Errorf("%w: -seq-interval must be positive", errUsage)
		}
		if c.seqScrollScreens < 0 {
			return c, fmt.Errorf("%w: -seq-scroll-screens must not be negative", errUsage)
		}
	} else if c.seqScrollScreens > 0 {
		return c, fmt.Errorf("%w: -seq-scroll-screens requires -screenshot-seq", errUsage)
	}
	if c.screenshot {
		if c.out == "" {
			return c, fmt.Errorf("%w: -screenshot requires -out <path.png>", errUsage)
		}
		if c.url == "" {
			return c, fmt.Errorf("%w: -screenshot requires -url", errUsage)
		}
	}
	if c.printPDF != "" {
		if fs.Lookup("backend").Value.String() == "" {
			c.backend = platform.Headless
		}
		if c.url == "" && len(fs.Args()) > 0 {
			c.url = fs.Args()[0]
		}
	}
	if c.appURL != "" && c.url == "" {
		c.url = c.appURL
	}
	if c.sessionFile == "" {
		c.sessionFile = filepath.Join(c.stateDir(), "session.json")
	}
	c.sessionActiveFile = filepath.Join(c.stateDir(), "session_active.json")
	c.cookieFile = filepath.Join(c.stateDir(), "cookies.json")
	c.bookmarkFile = filepath.Join(c.stateDir(), "bookmarks.json")
	c.historyFile = filepath.Join(c.stateDir(), "history.json")
	c.userscriptDir = filepath.Join(c.stateDir(), "userscripts")
	c.flagsFile = filepath.Join(c.stateDir(), "flags.json")
	c.permsFile = filepath.Join(c.stateDir(), "permissions.json")
	c.preferencesFile = filepath.Join(c.stateDir(), "preferences.json")
	return c, nil
}

func sceneSpec(name string) (paint.SceneSpec, error) {
	switch name {
	case "checkerboard":
		return paint.SceneSpec{Checkerboard: true, TextRuns: paint.DefaultTextRuns}, nil
	case "plain":
		return paint.SceneSpec{Checkerboard: false, TextRuns: paint.DefaultTextRuns}, nil
	}
	return paint.SceneSpec{}, fmt.Errorf("%w: unknown -scene %q (want checkerboard or plain)", errUsage, name)
}
