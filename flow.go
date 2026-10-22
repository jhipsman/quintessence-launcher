package main

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"quintessence-launcher/internal/client"
	"quintessence-launcher/internal/config"
)

// The error texts below are written for the player: they are shown as they are
// in the status line under Play.
var (
	errNoServer    = errors.New("No server is set. Enter the server address in Settings.")
	errBadServer   = errors.New("The server address is invalid. Use host:port, like 127.0.0.1:6000.")
	errNoName      = errors.New("Choose a player name in Settings first.")
	errBadName     = errors.New("Player names are 3 to 32 letters, digits, - or _.")
	errNoFolder    = errors.New("Choose your New World folder in Settings first.")
	errNoGameFound = errors.New("Couldn't find Bin64\\NewWorld.exe in your New World folder. Check it in Settings.")
	errUnsupported = errors.New("This New World version isn't supported. Quintessence needs build 1.400.6031.")
	errPatchFailed = errors.New("Couldn't patch the game, so it was not started. Check that New World is build 1.400.6031 and try again.")
)

// copyFolderName is the folder the game copy gets inside the folder the player
// picks for it, so the copy never spreads its files over that folder.
const copyFolderName = "QuintessenceClient"

// copyRoot is the folder of the isolated game copy for these settings.
func copyRoot(c config.Config) string {
	if c.CopyDir == "" {
		return client.IsolatedCopyDir(config.DataDir())
	}
	return c.CopyDir
}

// copyDirFor turns the folder the player picked for the copy into the copy's
// own folder: <picked>\QuintessenceClient, or the pick itself when it already is one.
func copyDirFor(picked string) string {
	p := filepath.Clean(picked)
	if strings.EqualFold(filepath.Base(p), copyFolderName) {
		return p
	}
	return filepath.Join(p, copyFolderName)
}

// within says whether path is dir or inside it.
func within(path, dir string) bool {
	return strings.HasPrefix(strings.ToLower(filepath.Clean(path))+`\`, strings.ToLower(filepath.Clean(dir))+`\`)
}

// Progress texts for the status line.
const (
	statusChecking  = "Checking game files…"
	statusPreparing = "Preparing game files (first time only)…"
	statusStarting  = "Starting…"
)

// gateway is the game server the client connects to.
type gateway struct {
	host string // host or IP, no port
	addr string // host:port, for -GatewayAddr
}

// serverFor picks the server: the one in the settings (host:port) or, when that
// is empty, the one the build was made for.
func serverFor(c config.Config) (gateway, error) {
	s := strings.TrimSpace(c.Server)
	if s == "" {
		return parseGateway(gatewayHost, gatewayPort)
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return gateway{}, errBadServer
	}
	return parseGateway(host, port)
}

// checkName validates a player name: it goes into the game as the persona, so
// it is kept to plain ASCII letters, digits, - and _.
func checkName(name string) error {
	if len(name) < 3 || len(name) > 32 {
		return errBadName
	}
	for _, c := range name {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
		if !ok {
			return errBadName
		}
	}
	return nil
}

// parseGateway checks a server host and port.
func parseGateway(host, port string) (gateway, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return gateway{}, errNoServer
	}
	if strings.ContainsAny(host, " :/\\\"'") {
		return gateway{}, errBadServer
	}
	p, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || p < 1 || p > 65535 {
		return gateway{}, errBadServer
	}
	return gateway{host: host, addr: net.JoinHostPort(host, strconv.Itoa(p))}, nil
}

// prepareGame turns the folder saved in the settings into the Bin64 folder the
// game will run from:
//
//  1. the folder is resolved (the install root, its Bin64 folder or NewWorld.exe);
//  2. NewWorld.exe must be the supported build (normalized sha256);
//  3. if the folder is an install of the game, the game never runs there: it
//     runs from an isolated copy in copyDir, which is built the first time and
//     reused while its exe and its assets link still match the install.
//
// Anything else (a folder that is already an isolated copy) runs in place:
// the patches only ever live in the process memory. status gets progress texts.
func prepareGame(gameDir, copyDir string, status func(string)) (string, error) {
	if status == nil {
		status = func(string) {}
	}
	if strings.TrimSpace(gameDir) == "" {
		return "", errNoFolder
	}
	root, binDir, err := client.ResolveGame(gameDir)
	if err != nil {
		return "", errNoGameFound
	}

	status(statusChecking)
	sum, _, err := client.NormalizedSHA256(filepath.Join(binDir, client.ExeName))
	switch {
	case errors.Is(err, client.ErrTooShort):
		return "", errUnsupported
	case err != nil:
		return "", fmt.Errorf("Couldn't read NewWorld.exe: %w", err)
	case sum != client.ExpectedSHA256:
		return "", errUnsupported
	}

	if !client.IsSteamInstall(binDir) {
		return binDir, nil
	}
	dest := copyDir
	if !client.CopyUpToDate(root, dest) {
		status(statusPreparing)
		if _, err := client.Prepare(client.PrepareOptions{GameDir: root, Dest: dest}); err != nil {
			return "", fmt.Errorf("Couldn't prepare the game files: %w", err)
		}
	}
	return filepath.Join(dest, "Bin64"), nil
}

// startError turns the error of client.Launch into the text the player sees. A
// failed patch is explained in a sentence; the technical detail is not shown.
func startError(err error) error {
	if errors.Is(err, client.ErrPatchFailed) {
		return errPatchFailed
	}
	return fmt.Errorf("Couldn't start the game: %w", err)
}
