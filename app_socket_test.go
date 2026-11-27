package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/oboo/moho/internal/mcp"
)

// TestSocketDir: on Windows the sockets of a data folder in AppData are
// under the profile, in a folder for each data folder, whatever case its
// letters are written in; any other data folder keeps them, as every data
// folder does elsewhere.
func TestSocketDir(t *testing.T) {
	if runtime.GOOS != "windows" {
		if d := socketDir("/home/me/.config/bowel"); d != "/home/me/.config/bowel" {
			t.Errorf("moved: %s", d)
		}
		t.Skip("the rest is Windows' paths")
	}
	d := windowsDirs{home: `C:\Users\me`, roaming: `C:\Users\me\AppData\Roaming`, local: `C:\Users\me\AppData\Local`, temp: `C:\Users\me\AppData\Local\Temp`}
	profile := `C:\Users\me\.bowel`
	def := windowsSocketDir(`C:\Users\me\AppData\Roaming\bowel`, d)
	if filepath.Dir(def) != profile || len(filepath.Base(def)) != 8 {
		t.Errorf("the default data folder's: %s", def)
	}
	if got := windowsSocketDir(`c:\users\ME\appdata\roaming\Bowel\`, d); got != def {
		t.Errorf("the same folder written otherwise: %s, want %s", got, def)
	}
	seen := map[string]bool{def: true}
	for _, dir := range []string{`C:\Users\me\AppData\Local\bowel`, `C:\Users\me\AppData\LocalLow\bowel`, `C:\Users\me\AppData\bowel`, `C:\Users\me\AppData\Roaming\bowel\x`} {
		got := windowsSocketDir(dir, d)
		if filepath.Dir(got) != profile || seen[got] {
			t.Errorf("%s: %s", dir, got)
		}
		seen[got] = true
	}
	for _, dir := range []string{`C:\Users\me\AppData\Local\Temp\mh123`, `C:\ProgramData\bowel`, `D:\bowel`, `C:\Users\me\AppDataX\bowel`, `C:\Users\me\bowel`} {
		if got := windowsSocketDir(dir, d); got != dir {
			t.Errorf("%s moved to %s", dir, got)
		}
	}
	// Roaming that an administrator moved to a share is seen through a
	// copy all the same.
	r := d
	r.roaming = `\\server\home\me\AppData\Roaming`
	if got := windowsSocketDir(`\\server\home\me\AppData\Roaming\bowel`, r); filepath.Dir(got) != profile {
		t.Errorf("redirected: %s", got)
	}
	// No profile, nowhere better.
	if got := windowsSocketDir(`C:\Users\me\AppData\Roaming\bowel`, windowsDirs{}); got != `C:\Users\me\AppData\Roaming\bowel` {
		t.Errorf("no profile: %s", got)
	}
}

// TestSocketsLeaveAppData: with the user's folders of a made-up profile, a
// data folder in Roaming has both sockets in one folder under the profile,
// made when the first is served, and bowel 0.4.9's place in the data
// folder is still looked at: by running, and by dropOlderSocket, which
// keeps a socket that answers and removes one that doesn't.
func TestSocketsLeaveAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		if p := olderCLISocket("/home/me/.config/bowel"); p != "" {
			t.Errorf("an older place: %s", p)
		}
		t.Skip("Windows' AppData")
	}
	home, err := os.MkdirTemp("", "bh") // short: unix socket paths are limited
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	// The made-up profile is in the real temporary folder: one of its own.
	t.Setenv("TMP", filepath.Join(home, "AppData", "Local", "Temp"))
	t.Setenv("TEMP", filepath.Join(home, "AppData", "Local", "Temp"))
	data := filepath.Join(home, "AppData", "Roaming", "bowel")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}

	cliP, mcpP := cliSocket(data), mcpSocket(data)
	if dir := filepath.Dir(cliP); filepath.Dir(dir) != filepath.Join(home, ".bowel") || filepath.Dir(mcpP) != dir {
		t.Fatalf("sockets: %s, %s", cliP, mcpP)
	}
	if p := olderCLISocket(data); p != filepath.Join(data, "cli.sock") {
		t.Errorf("older cli.sock: %s", p)
	}
	if p := olderMCPSocket(data); p != filepath.Join(data, "mcp.sock") {
		t.Errorf("older mcp.sock: %s", p)
	}
	// A data folder outside AppData has no older place.
	if p := olderMCPSocket(home); p != "" {
		t.Errorf("outside AppData: %s", p)
	}

	l, err := mcp.Listen(cliP)
	if err != nil {
		t.Fatal(err)
	}
	if !running(data) {
		t.Error("served under the profile, not running")
	}
	l.Close()
	if running(data) {
		t.Error("running with nothing served")
	}

	old, err := mcp.Listen(olderCLISocket(data))
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if !running(data) {
		t.Error("an older bowel serving the data folder, not running")
	}
	dropOlderSocket(olderCLISocket(data))
	if _, err := os.Stat(olderCLISocket(data)); err != nil {
		t.Errorf("a socket that answers was dropped: %v", err)
	}
	if err := os.WriteFile(olderMCPSocket(data), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dropOlderSocket(olderMCPSocket(data))
	if _, err := os.Stat(olderMCPSocket(data)); !os.IsNotExist(err) {
		t.Errorf("what a crash left is still there: %v", err)
	}
}
