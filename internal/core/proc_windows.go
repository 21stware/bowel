//go:build windows

package core

import (
	"os/exec"

	"github.com/oboo/moho/internal/nowindow"
)

func setProcAttr(cmd *exec.Cmd) { nowindow.Hide(cmd) }

func interrupt(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// killStale is not implemented on Windows yet.
func killStale(string) {}
