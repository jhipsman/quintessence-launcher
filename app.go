package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"quintessence-launcher/internal/client"
	"quintessence-launcher/internal/config"
)

// Events sent to the frontend (runtime.EventsOn on the JS side).
const (
	evBusy   = "app:busy"    // bool: Play is running
	evStatus = "play:status" // string: the status line under Play
)

// App is the object exposed to the frontend (window.go.main.App).
type App struct {
	ctx context.Context

	mu           sync.Mutex
	searchCancel context.CancelFunc // folder search in progress (CancelSearch)
	cfg          config.Config
	busy         bool

	// emit sends an event to the frontend. It is a no-op until startup, and tests replace it.
	emit func(event string, data ...any)
}

// PlayResult is the outcome of Launch. Error is a sentence for the player.
type PlayResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// SearchResult is the outcome of SearchInstalls.
type SearchResult struct {
	Installs  []client.Install `json:"installs"`
	Complete  bool             `json:"complete"`  // false: the time cap hit before every folder was checked
	Cancelled bool             `json:"cancelled"` // the player cancelled the search
}

func NewApp() *App { return &App{emit: func(string, ...any) {}} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emit = func(event string, data ...any) { runtime.EventsEmit(ctx, event, data...) }
	cfg, err := config.Load()
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	if err != nil {
		runtime.LogWarning(ctx, "config: "+err.Error())
	}
}

func (a *App) setBusy(b bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if b && a.busy {
		return false
	}
	a.busy = b
	a.emit(evBusy, b)
	return true
}

func (a *App) setStatus(s string) { a.emit(evStatus, s) }

func (a *App) snapshot() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// ---- bindings -----------------------------------------------------------

// GetVersion returns the launcher version.
func (a *App) GetVersion() string { return Version }

// GetConfig returns the current settings.
func (a *App) GetConfig() config.Config { return a.snapshot() }

// SaveConfig saves the settings. The player name and the server address are
// checked here (an empty server means the build's own). The New World folder
// is only checked for existing; whether it really is New World is checked when the player
// presses Play. The copy folder becomes <picked>\QuintessenceClient (see copyDirFor)
// and has to be outside the game. When the copy moves, the old one is deleted
// (client.RemoveCopy) and the next Play builds it in the new folder.
func (a *App) SaveConfig(c config.Config) (config.Config, error) {
	c.PlayerName = strings.TrimSpace(c.PlayerName)
	if c.PlayerName != "" {
		if err := checkName(c.PlayerName); err != nil {
			return a.snapshot(), err
		}
	}
	c.Server = strings.TrimSpace(c.Server)
	if c.Server != "" {
		if _, err := serverFor(c); err != nil {
			return a.snapshot(), err
		}
	}
	c.GameDir = cleanPath(c.GameDir)
	if c.GameDir != "" {
		if _, err := os.Stat(c.GameDir); err != nil {
			return a.snapshot(), errors.New("That folder doesn't exist.")
		}
	}
	c.CopyDir = cleanPath(c.CopyDir)
	if c.CopyDir != "" {
		if !filepath.IsAbs(c.CopyDir) {
			return a.snapshot(), errors.New(`Choose a full folder path for the game copy, like D:\Games.`)
		}
		c.CopyDir = copyDirFor(c.CopyDir)
		if st, err := os.Stat(filepath.Dir(c.CopyDir)); err != nil || !st.IsDir() {
			return a.snapshot(), errors.New("The game copy folder doesn't exist.")
		}
		if st, err := os.Stat(c.CopyDir); err == nil && !st.IsDir() {
			return a.snapshot(), fmt.Errorf("%s is a file. Choose another folder for the game copy.", c.CopyDir)
		}
		if client.IsSteamInstall(filepath.Join(c.CopyDir, "Bin64")) {
			return a.snapshot(), errors.New("The game copy can't go inside a game install. Choose another folder.")
		}
		if root, _, err := client.ResolveGame(c.GameDir); err == nil && (within(c.CopyDir, root) || within(root, c.CopyDir)) {
			return a.snapshot(), errors.New("The game copy can't go inside your New World folder. Choose another folder.")
		}
	}
	a.mu.Lock()
	old := copyRoot(a.cfg)
	a.cfg = c
	a.mu.Unlock()
	if err := config.Save(c); err != nil {
		return c, fmt.Errorf("Couldn't save the settings: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(old), filepath.Clean(copyRoot(c))) {
		if _, err := os.Stat(old); err == nil {
			if err := client.RemoveCopy(old); err != nil { // the old copy only costs disk space
				runtime.LogWarning(a.ctx, "old game copy: "+err.Error())
			}
		}
	}
	return c, nil
}

// DefaultServer is the server this build was made for as host:port, or "" if none.
func (a *App) DefaultServer() string {
	gw, err := parseGateway(gatewayHost, gatewayPort)
	if err != nil {
		return ""
	}
	return gw.addr
}

// DefaultCopyDir is where the game copy lives when no folder is picked.
func (a *App) DefaultCopyDir() string { return client.IsolatedCopyDir(config.DataDir()) }

// PickCopyDir opens the folder picker for the game copy. The copy goes in a
// QuintessenceClient folder inside the folder picked here.
func (a *App) PickCopyDir() (string, error) {
	start := filepath.Dir(a.snapshot().CopyDir)
	if _, err := os.Stat(start); err != nil || a.snapshot().CopyDir == "" {
		start = ""
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Folder for the game copy",
		DefaultDirectory: start,
	})
}

// cleanPath trims spaces and the quotes Explorer puts around "Copy as path".
func cleanPath(p string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(p), `"`))
}

// PickGameDir opens the folder picker for the New World folder.
func (a *App) PickGameDir() (string, error) {
	start := a.snapshot().GameDir
	if _, err := os.Stat(start); err != nil {
		start = ""
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "New World folder",
		DefaultDirectory: start,
	})
}

// SearchInstalls looks for New World installs (Steam libraries and a scan of
// the fixed drives). It blocks until it is done, cancelled with CancelSearch or
// the time cap hits; the screen awaits it without freezing.
func (a *App) SearchInstalls() (SearchResult, error) {
	a.mu.Lock()
	if a.searchCancel != nil {
		a.mu.Unlock()
		return SearchResult{}, errors.New("A search is already running.")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.searchCancel = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.searchCancel = nil
		a.mu.Unlock()
		cancel()
	}()

	installs, complete, err := client.Search(ctx, client.SearchOptions{Skip: []string{config.DataDir(), copyRoot(a.snapshot())}})
	if err != nil { // only a cancelled context
		return SearchResult{Installs: []client.Install{}, Cancelled: true}, nil
	}
	if installs == nil {
		installs = []client.Install{}
	}
	return SearchResult{Installs: installs, Complete: complete}, nil
}

// CancelSearch stops the search in progress.
func (a *App) CancelSearch() {
	a.mu.Lock()
	cancel := a.searchCancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Launch starts the game with the in-memory patches. It refuses without a
// player name. Progress goes out as evStatus events; the result carries the
// error text, if any.
func (a *App) Launch() PlayResult {
	if !a.setBusy(true) {
		return PlayResult{Error: "Something is already in progress."}
	}
	defer a.setBusy(false)
	if err := a.play(); err != nil {
		return PlayResult{Error: err.Error()}
	}
	return PlayResult{OK: true}
}

func (a *App) play() error {
	cfg := a.snapshot()
	gw, err := serverFor(cfg)
	if err != nil {
		return err
	}
	if cfg.PlayerName == "" {
		return errNoName
	}
	if err := checkName(cfg.PlayerName); err != nil {
		return err
	}
	binDir, err := prepareGame(cfg.GameDir, copyRoot(cfg), a.setStatus)
	if err != nil {
		return err
	}
	a.setStatus(statusStarting)
	_, err = client.Launch(client.LaunchOptions{
		BinDir:      binDir,
		GatewayAddr: gw.addr,
		PingHost:    gw.host,
		Persona:     cfg.PlayerName,
	})
	if err != nil { // Launch already killed the process if a patch failed
		return startError(err)
	}
	return nil
}
