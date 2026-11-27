package main

// Where the app's two sockets (cli.sock, mcp.sock) are. On every system
// they are in the data folder, unless its path is too long for a unix
// socket (cliSocket, mcpSocket). On Windows a data folder in the user's
// AppData, as the default one is (%APPDATA%\bowel), keeps them elsewhere.
//
// Windows shows AppData to the processes of an MSIX-packaged app, such as
// the terminals in Claude's desktop app, through a copy of their own
// (file-system virtualization), and connecting to a unix socket through
// that view fails with WSAEINVAL, "An invalid argument was supplied":
// whether the app listening runs outside the package or is the very
// process that connects. Measured on Windows 11 (26300) from a terminal of
// Claude's desktop app, for Roaming, Local, LocalLow and AppData itself.
// The user's profile outside AppData, and the temporary folder in it, are
// shown as they are, and a socket in either connects across the two views,
// both ways. So the sockets of such a data folder go in a folder of their
// own under the profile, %USERPROFILE%\.bowel\<8 hex digits>, named after
// the data folder so that two (the app's and a daemon's) don't share one.
// Not the temporary folder: cleaning it up can take the socket from an app
// that has run for weeks, and where it is follows TMP and TEMP, which a
// shell can change.

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// socketDir is the folder of the sockets for the data folder dataDir.
func socketDir(dataDir string) string {
	if runtime.GOOS != "windows" {
		return dataDir
	}
	home, _ := os.UserHomeDir()
	return windowsSocketDir(dataDir, windowsDirs{
		home: home, roaming: os.Getenv("APPDATA"), local: os.Getenv("LOCALAPPDATA"), temp: os.TempDir(),
	})
}

// windowsDirs are the user's folders windowsSocketDir goes by.
type windowsDirs struct{ home, roaming, local, temp string }

// windowsSocketDir is socketDir on Windows: under the profile for a data
// folder in AppData, outside its temporary folder; dataDir otherwise.
func windowsSocketDir(dataDir string, d windowsDirs) string {
	if d.home == "" || within(dataDir, d.temp) {
		return dataDir
	}
	// Roaming and Local are in the profile's AppData unless the
	// administrator moved them, and are seen through a copy wherever
	// they are.
	for _, root := range []string{filepath.Join(d.home, "AppData"), d.roaming, d.local} {
		if within(dataDir, root) {
			sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dataDir))))
			return filepath.Join(d.home, ".bowel", fmt.Sprintf("%x", sum[:4]))
		}
	}
	return dataDir
}

// within says whether dir is root or inside it. On Windows, letters'
// case does not tell paths apart (filepath.Rel).
func within(dir, root string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, dir)
	return err == nil && filepath.IsLocal(rel)
}

// olderSocket is where bowel 0.4.9 and before served the socket that is
// now at now, old, or "" when it is the same place. Before, the sockets
// were in the data folder on Windows too.
func olderSocket(now, old string) string {
	if old == now {
		return ""
	}
	return old
}

// dropOlderSocket removes a socket an older bowel left at path (olderSocket)
// when nothing answers on it, as Listen does at its own. A terminal of a
// packaged app sees such a file as a socket it can't reach, whether or not
// anything listens (WSAEINVAL), so the command line would go on saying
// that bowel might be running.
func dropOlderSocket(path string) {
	if path == "" {
		return
	}
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Close()
		return
	}
	_ = os.Remove(path)
}
