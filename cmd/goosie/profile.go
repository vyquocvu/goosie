package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/flags"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/perms"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/userscript"
)

// profile holds the stores every window of one invocation shares: one cookie
// jar, one history, one bookmark store, one localStorage per origin, one
// permissions store, one glyph atlas. The windowManager creates it once and
// hands the same pointers to each framePath, so a bookmark added in one window
// is visible in every other. It is an owned value, never a package global.
type profile struct {
	client                net.HTTP
	privateClient         net.HTTP
	fonts                 *raster.Fonts
	bookmarks             *bookmarks.Store
	history               *history.Store
	downloads             *download.Manager
	storage               *js.OriginStorageRegistry
	incognitoStorage      *js.OriginStorageRegistry
	broadcastHub          *js.BroadcastHub
	incognitoBroadcastHub *js.BroadcastHub
	perms                 *perms.Manager
	preferences           preferences.Settings
	flags                 *flags.Manager
	userscripts           *userscript.Manager
	theme                 string
}

// buildProfile creates the shared stores once. It is the profile-level half of
// the old build: preferences/download-dir resolution, fonts, HTTP clients,
// bookmark/history load, and the registry/manager constructors, plus the
// userscripts/flags/perms Load calls that used to run per build.
func buildProfile(c config) (*profile, error) {
	profilePreferences, prefErr := preferences.Load(c.preferencesFile)
	if prefErr != nil {
		fmt.Fprintf(os.Stderr, "goosie: preferences: %v\n", prefErr)
	}
	effectiveDir, err := effectiveDownloadDir(c.downloadDir, c.downloadDirExplicit, profilePreferences.DownloadDir)
	if err != nil {
		return nil, fmt.Errorf("goosie: download directory: %w", err)
	}
	profilePreferences.DownloadDir = effectiveDir

	fonts, err := raster.NewFonts()
	if err != nil {
		return nil, fmt.Errorf("goosie: %w", err)
	}

	var client net.HTTP
	if c.private {
		client = net.DefaultPrivateClient()
	} else {
		persistent, cerr := net.DefaultClientWithCookies(c.cookieFile)
		if cerr != nil {
			fmt.Fprintf(os.Stderr, "goosie: cookies: %v\n", cerr)
			client = net.DefaultClient()
		} else {
			client = persistent
		}
	}

	var bookmarkStore *bookmarks.Store
	var historyStore *history.Store
	if !c.private {
		bookmarkStore, err = bookmarks.Load(c.bookmarkFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goosie: bookmarks: %v\n", err)
			bookmarkStore = nil
		}
		historyStore, err = history.Load(c.historyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goosie: history: %v\n", err)
			historyStore = nil
		}
	}

	p := &profile{
		client:                client,
		privateClient:         net.DefaultPrivateClient(),
		fonts:                 fonts,
		bookmarks:             bookmarkStore,
		history:               historyStore,
		downloads:             download.NewManager(effectiveDir),
		storage:               js.NewOriginStorageRegistry(filepath.Join(c.stateDir(), "storage"), false),
		incognitoStorage:      js.NewOriginStorageRegistry("", true),
		broadcastHub:          js.NewBroadcastHub(),
		incognitoBroadcastHub: js.NewBroadcastHub(),
		theme:                 c.theme,
		userscripts:           userscript.NewManager(c.userscriptDir),
		flags:                 flags.NewManager(c.flagsFile),
		perms:                 perms.NewManager(c.permsFile),
		preferences:           profilePreferences,
	}
	if p.theme == "" {
		p.theme = "system"
	}
	if !c.paced() && p.userscripts != nil {
		_ = p.userscripts.Load()
	}
	if !c.paced() && p.flags != nil {
		_ = p.flags.Load()
	}
	if !c.paced() && p.perms != nil {
		_ = p.perms.Load()
	}
	return p, nil
}

// close releases the shared HTTP clients once, after every window is gone.
func (p *profile) close() {
	if p.client != nil {
		if err := p.client.Close(); err != nil {
			fmt.Fprintln(os.Stderr, programErrorLine(err))
		}
	}
	if p.privateClient != nil {
		_ = p.privateClient.Close()
	}
}
