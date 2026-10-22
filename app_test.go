package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"quintessence-launcher/internal/client"
	"quintessence-launcher/internal/config"
)

// newTestApp makes an App with a private data directory and a recorded event log.
func newTestApp(t *testing.T) (*App, *[]string) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	a := NewApp()
	a.ctx = context.Background()
	var mu sync.Mutex
	var events []string
	a.emit = func(event string, data ...any) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	return a, &events
}

func setBuildSettings(t *testing.T, host, port string) {
	t.Helper()
	oldH, oldP := gatewayHost, gatewayPort
	gatewayHost, gatewayPort = host, port
	t.Cleanup(func() { gatewayHost, gatewayPort = oldH, oldP })
}

func TestParseGateway(t *testing.T) {
	gw, err := parseGateway(" play.example.test ", "6000")
	if err != nil || gw.host != "play.example.test" || gw.addr != "play.example.test:6000" {
		t.Fatalf("ok case: %+v %v", gw, err)
	}
	if _, err := parseGateway("", "6000"); err != errNoServer {
		t.Fatalf("empty host: %v", err)
	}
	if _, err := parseGateway("   ", "6000"); err != errNoServer {
		t.Fatalf("blank host: %v", err)
	}
	for _, port := range []string{"", "0", "-1", "65536", "70000", "abc", "60 00"} {
		if _, err := parseGateway("play.example.test", port); err != errBadServer {
			t.Fatalf("port %q: %v", port, err)
		}
	}
	for _, host := range []string{"a b", "host:6000", "http://host", `ho"st`} {
		if _, err := parseGateway(host, "6000"); err != errBadServer {
			t.Fatalf("host %q: %v", host, err)
		}
	}
	if gw, err := parseGateway("play.example.test", "65535"); err != nil || gw.addr != "play.example.test:65535" {
		t.Fatalf("port 65535: %+v %v", gw, err)
	}
}

func TestServerFor(t *testing.T) {
	setBuildSettings(t, "build.example.test", "7000")
	if gw, err := serverFor(config.Config{}); err != nil || gw.addr != "build.example.test:7000" {
		t.Fatalf("build default: %+v %v", gw, err)
	}
	if gw, err := serverFor(config.Config{Server: " 127.0.0.1:6000 "}); err != nil || gw.host != "127.0.0.1" || gw.addr != "127.0.0.1:6000" {
		t.Fatalf("setting: %+v %v", gw, err)
	}
	for _, bad := range []string{"127.0.0.1", "host:", ":6000", "host:abc", "a b:6000", "http://host:6000"} {
		if _, err := serverFor(config.Config{Server: bad}); err != errBadServer && err != errNoServer {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	setBuildSettings(t, "", "")
	if _, err := serverFor(config.Config{}); err != errNoServer {
		t.Fatalf("no server anywhere: %v", err)
	}
}

func TestCheckName(t *testing.T) {
	for _, ok := range []string{"Bob", "alice_99", "Dark-Knight", strings.Repeat("x", 32)} {
		if err := checkName(ok); err != nil {
			t.Fatalf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "ab", strings.Repeat("x", 33), "two words", "émile", "a;b", "tab\tx"} {
		if err := checkName(bad); err != errBadName {
			t.Fatalf("%q accepted: %v", bad, err)
		}
	}
}

func TestSaveConfigNameAndServer(t *testing.T) {
	a, _ := newTestApp(t)
	got, err := a.SaveConfig(config.Config{PlayerName: " Alice ", Server: " 127.0.0.1:6000 "})
	if err != nil || got.PlayerName != "Alice" || got.Server != "127.0.0.1:6000" {
		t.Fatalf("save: %+v %v", got, err)
	}
	if c, err := config.Load(); err != nil || c.PlayerName != "Alice" || c.Server != "127.0.0.1:6000" {
		t.Fatalf("on disk: %+v %v", c, err)
	}
	// refused, and the previous values stay
	for name, c := range map[string]config.Config{
		"bad name":   {PlayerName: "a b", Server: "127.0.0.1:6000"},
		"bad server": {PlayerName: "Alice", Server: "127.0.0.1"},
	} {
		if got, err := a.SaveConfig(c); err == nil || got.PlayerName != "Alice" || got.Server != "127.0.0.1:6000" {
			t.Fatalf("%s: accepted (%+v %v)", name, got, err)
		}
	}
	// both may be cleared
	if got, err := a.SaveConfig(config.Config{}); err != nil || got.PlayerName != "" || got.Server != "" {
		t.Fatalf("clear: %+v %v", got, err)
	}
}

func TestSaveConfig(t *testing.T) {
	a, _ := newTestApp(t)
	dir := t.TempDir()

	got, err := a.SaveConfig(config.Config{GameDir: `  "` + dir + `"  `})
	if err != nil || got.GameDir != dir {
		t.Fatalf("save: %+v %v", got, err)
	}
	if a.GetConfig().GameDir != dir {
		t.Fatalf("GetConfig = %+v", a.GetConfig())
	}
	if c, err := config.Load(); err != nil || c.GameDir != dir {
		t.Fatalf("on disk: %+v %v", c, err)
	}

	// a folder that does not exist is refused and the previous value stays
	got, err = a.SaveConfig(config.Config{GameDir: filepath.Join(dir, "nope")})
	if err == nil || got.GameDir != dir || a.GetConfig().GameDir != dir {
		t.Fatalf("missing folder: %+v %v", got, err)
	}
	// nothing else is validated: any existing folder is fine, even a wrong one
	if _, err := a.SaveConfig(config.Config{GameDir: os.TempDir()}); err != nil {
		t.Fatalf("an existing folder must be accepted: %v", err)
	}
	// empty clears the setting
	if got, err := a.SaveConfig(config.Config{}); err != nil || got.GameDir != "" {
		t.Fatalf("clear: %+v %v", got, err)
	}
}

func TestSaveConfigCopyDir(t *testing.T) {
	a, _ := newTestApp(t)
	if got := copyRoot(a.GetConfig()); got != a.DefaultCopyDir() || got != client.IsolatedCopyDir(config.DataDir()) {
		t.Fatalf("default copy folder = %q", got)
	}
	base := t.TempDir()

	// the copy goes in a QuintessenceClient folder inside the picked one
	got, err := a.SaveConfig(config.Config{CopyDir: `"` + base + `"`})
	want := filepath.Join(base, copyFolderName)
	if err != nil || got.CopyDir != want || copyRoot(a.GetConfig()) != want {
		t.Fatalf("save: %+v %v", got, err)
	}
	// picking that QuintessenceClient folder again keeps it as it is
	if got, err := a.SaveConfig(config.Config{CopyDir: want}); err != nil || got.CopyDir != want {
		t.Fatalf("save again: %+v %v", got, err)
	}
	if c, err := config.Load(); err != nil || c.CopyDir != want {
		t.Fatalf("on disk: %+v %v", c, err)
	}

	// refused, and the previous value stays
	mkGame := func(root string) string {
		if err := os.MkdirAll(filepath.Join(root, "Bin64"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "Bin64", client.ExeName), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return root
	}
	game := mkGame(filepath.Join(t.TempDir(), "New World"))
	nested := mkGame(filepath.Join(t.TempDir(), copyFolderName, "New World"))
	steam := filepath.Join(t.TempDir(), "steamapps", "common")
	if err := os.MkdirAll(steam, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(steam, copyFolderName), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]config.Config{
		"relative":      {CopyDir: `Games`},
		"missing":       {CopyDir: filepath.Join(base, "nope", "deeper")},
		"steam library": {CopyDir: steam},
		"in the game":   {GameDir: game, CopyDir: game},
		"around a game": {GameDir: nested, CopyDir: filepath.Dir(filepath.Dir(nested))},
		"is a file":     {CopyDir: filepath.Join(steam, copyFolderName)},
	} {
		if got, err := a.SaveConfig(c); err == nil || got.CopyDir != want || a.GetConfig().CopyDir != want {
			t.Fatalf("%s: accepted (%+v %v)", name, got, err)
		}
	}

	// moving the copy deletes the old one, never the game's assets behind it
	gameAssets := filepath.Join(game, "assets")
	if err := os.MkdirAll(gameAssets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameAssets, "game.pak"), []byte("pak"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(want, "Bin64"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(want, "Bin64", client.ExeName), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.CreateJunction(filepath.Join(want, "assets"), gameAssets); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if got, err := a.SaveConfig(config.Config{CopyDir: other}); err != nil || got.CopyDir != filepath.Join(other, copyFolderName) {
		t.Fatalf("move: %+v %v", got, err)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("the old copy is still there (%v)", err)
	}
	if got, err := os.ReadFile(filepath.Join(gameAssets, "game.pak")); err != nil || string(got) != "pak" {
		t.Fatalf("moving the copy touched the game's assets: %q %v", got, err)
	}
	// empty goes back to the default
	if got, err := a.SaveConfig(config.Config{}); err != nil || got.CopyDir != "" || copyRoot(got) != a.DefaultCopyDir() {
		t.Fatalf("clear: %+v %v", got, err)
	}
}

func TestPrepareGameErrors(t *testing.T) {
	data := filepath.Join(t.TempDir(), copyFolderName)
	if _, err := prepareGame("", data, nil); err != errNoFolder {
		t.Fatalf("empty: %v", err)
	}
	if _, err := prepareGame("   ", data, nil); err != errNoFolder {
		t.Fatalf("blank: %v", err)
	}
	if _, err := prepareGame(filepath.Join(t.TempDir(), "nope"), data, nil); err != errNoGameFound {
		t.Fatalf("missing folder: %v", err)
	}
	if _, err := prepareGame(t.TempDir(), data, nil); err != errNoGameFound {
		t.Fatalf("folder without the exe: %v", err)
	}

	// a fake exe is not the supported build, in the root and through Bin64
	root := filepath.Join(t.TempDir(), "New World")
	bin := filepath.Join(root, "Bin64")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, client.ExeName), []byte("not the game"), 0o644); err != nil {
		t.Fatal(err)
	}
	var statuses []string
	for _, in := range []string{root, bin} {
		if _, err := prepareGame(in, data, func(s string) { statuses = append(statuses, s) }); err != errUnsupported {
			t.Fatalf("fake exe (%s): %v", in, err)
		}
	}
	if len(statuses) == 0 || statuses[0] != statusChecking {
		t.Fatalf("statuses = %v", statuses)
	}
	if want := "This New World version isn't supported. Quintessence needs build 1.400.6031."; errUnsupported.Error() != want {
		t.Fatalf("unsupported text: %q", errUnsupported)
	}
	// nothing was built in the copy folder for a refused build
	if _, err := os.Stat(data); err == nil {
		t.Fatal("the isolated copy must not be created for an unsupported build")
	}
}

func TestLaunchRefusals(t *testing.T) {
	a, events := newTestApp(t)

	// no server compiled in and none in the settings
	setBuildSettings(t, "", "")
	if r := a.Launch(); r.OK || r.Error != errNoServer.Error() {
		t.Fatalf("no server: %+v", r)
	}

	// server, but no player name: Launch refuses on its own, whatever the screen does
	setBuildSettings(t, "play.example.test", "6000")
	if r := a.Launch(); r.OK || r.Error != errNoName.Error() {
		t.Fatalf("no name: %+v", r)
	}

	// a name, no folder
	if _, err := a.SaveConfig(config.Config{PlayerName: "Alice"}); err != nil {
		t.Fatal(err)
	}
	if r := a.Launch(); r.OK || r.Error != errNoFolder.Error() {
		t.Fatalf("no folder: %+v", r)
	}
	// the busy lock is released after every attempt
	if a.busy {
		t.Fatal("still busy after Launch returned")
	}
	if len(*events) == 0 {
		t.Fatal("no busy events were emitted")
	}
}

func TestLaunchIsSerialized(t *testing.T) {
	a, _ := newTestApp(t)
	if !a.setBusy(true) {
		t.Fatal("first setBusy must work")
	}
	if r := a.Launch(); r.OK || r.Error == "" {
		t.Fatalf("a second Launch while busy: %+v", r)
	}
	if a.setBusy(true) {
		t.Fatal("setBusy must refuse while busy")
	}
	a.setBusy(false)
}

func TestStartError(t *testing.T) {
	patch := fmt.Errorf("%w: %w", client.ErrPatchFailed, errors.New("[dtls] unexpected bytes at the patch site (different build?): 0F 84"))
	if got := startError(patch); got != errPatchFailed || strings.Contains(got.Error(), "dtls") {
		t.Fatalf("patch failure: %v", got)
	}
	other := errors.New("the client exited right after starting (exit code 1 / 0x1)")
	if got := startError(other); got.Error() != "Couldn't start the game: the client exited right after starting (exit code 1 / 0x1)" || !errors.Is(got, other) {
		t.Fatalf("other failure: %v", got)
	}
}
