package main

import (
	"cmp"
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
// first, then by name. On macOS an application is a bundle, so a running
// program outside one (a daemon, a shell) is left out, and so are the
// system's own bundles (/System/Library); a program with connections is
// offered whatever it is, since its rule is what was asked for.
func appChoices(running, connected []string) []appChoice {
	var out []appChoice
	add := func(p string, conn bool) {
		name := routes.AppOf(p)
		if name == "" {
			return
		}
		bundle := bundleOf(p, name)
		if !conn && runtime.GOOS == "darwin" && (bundle == "" || systemBundle(bundle)) {
			return
		}
		i := slices.IndexFunc(out, func(c appChoice) bool { return strings.EqualFold(c.Name, name) })
		if i < 0 {
			out = append(out, appChoice{Name: name, Bundle: bundle})
			i = len(out) - 1
		}
		if out[i].Bundle == "" {
			out[i].Bundle = bundle
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
