package main

import (
	"cmp"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"

	"github.com/oboo/moho/internal/procinfo"
	"github.com/oboo/moho/internal/routes"
)

// appChoice is an application the rule editor offers for a rule of kind
// 应用: one running now, or with connections through bowel lately.
type appChoice struct {
	Name   string // as a rule names it (routes.AppOf)
	Bundle string // its bundle's folder on macOS, where its icon is; else ""
	Conns  int    // its connections in the Log: open, or closed lately
}

// runningApps are the applications of this user's running programs and of
// the connections the Log has, open or closed lately.
func (a *App) runningApps() []appChoice {
	var conns []string
	for _, c := range a.connections() {
		conns = append(conns, c.Process)
	}
	return appChoices(procinfo.Running(), conns)
}

// appChoices are the applications of the running programs and of the
// connections' (their paths, or names), each once: those with connections
// first, then by name. A running program is offered only when offered says
// so; a program with connections is offered whatever it is, since its rule
// is what was asked for. bowel itself is never offered. Of two bundles of
// the same name, one where apps are installed is shown, by its spelling:
// Claude Code keeps a claude.app of its own beside /Applications/Claude.app.
func appChoices(running, connected []string) []appChoice {
	var out []appChoice
	add := func(p string, conn bool) {
		name := routes.AppOf(p)
		if name == "" || strings.EqualFold(name, ownApp) {
			return
		}
		bundle := bundleOf(p, name)
		if !conn && !offered(p, bundle) {
			return
		}
		i := slices.IndexFunc(out, func(c appChoice) bool { return strings.EqualFold(c.Name, name) })
		if i < 0 {
			out = append(out, appChoice{Name: name, Bundle: bundle})
			i = len(out) - 1
		}
		if bundle != "" && (out[i].Bundle == "" || installed(bundle) && !installed(out[i].Bundle)) {
			out[i].Name, out[i].Bundle = name, bundle
		}
		if conn {
			out[i].Conns++
		}
	}
	for _, p := range connected {
		add(p, true)
	}
	for _, p := range running {
		add(p, false)
	}
	slices.SortFunc(out, func(a, b appChoice) int {
		if (a.Conns > 0) != (b.Conns > 0) {
			if a.Conns > 0 {
				return -1
			}
			return 1
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

// bundleOf is the folder of the bundle named name that p is in, "" when
// name is not a bundle's (or p is not a path).
func bundleOf(p, name string) string {
	if !strings.HasSuffix(strings.ToLower(name), ".app") {
		return ""
	}
	p = strings.ReplaceAll(p, `\`, "/")
	i := strings.Index(strings.ToLower(p+"/"), "/"+strings.ToLower(name)+"/")
	if i < 0 {
		return ""
	}
	return p[:i+1+len(name)]
}

// ownApp is bowel's own application, as a rule would name it.
var ownApp = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return routes.AppOf(exe)
}()

// offered says whether a running program is offered without connections.
// On macOS an application is a bundle, so a program outside one (a daemon,
// a shell) is not, nor are the system's own bundles (/System/Library) and
// those inside a framework (Homebrew's Python.app). On Linux the system's
// helpers and the shells are left out. On Windows none is, until the list
// has been tried on a Windows machine: there only apps with connections.
func offered(p, bundle string) bool {
	switch runtime.GOOS {
	case "darwin":
		return bundle != "" && !systemBundle(bundle) && !strings.Contains(strings.ToLower(bundle), ".framework/")
	case "windows":
		return false
	case "linux":
		for _, dir := range []string{"/usr/lib/systemd/", "/lib/systemd/", "/usr/libexec/"} {
			if strings.HasPrefix(p, dir) {
				return false
			}
		}
		return !slices.Contains(shells, path.Base(p))
	}
	return true
}

// shells are the common shells, programs no one gives a rule of their own.
var shells = []string{"sh", "bash", "dash", "zsh", "fish", "ksh", "mksh", "tcsh", "csh", "nu", "elvish"}

// installed says whether a bundle is where apps are installed: the
// Applications folders of the system, the computer and the user.
func installed(b string) bool {
	dirs := []string{"/Applications/", "/System/Applications/"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, home+"/Applications/")
	}
	for _, dir := range dirs {
		if strings.HasPrefix(b, dir) {
			return true
		}
	}
	return false
}

// systemBundle says whether a bundle is part of macOS itself, where no one
// looks for an app to give a rule: its agents and services. The apps it
// comes with (Safari, Mail) live elsewhere.
func systemBundle(b string) bool {
	for _, dir := range []string{"/System/Library/", "/Library/Apple/", "/usr/"} {
		if strings.HasPrefix(b, dir) {
			return true
		}
	}
	return false
}
