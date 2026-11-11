//go:build !darwin

package core

import "io"

// startCore runs the core as a child of the app.
func startCore(bin string, args []string, dir string, env []string, out io.Writer) (Process, error) {
	return startChild(bin, args, dir, env, out)
}
