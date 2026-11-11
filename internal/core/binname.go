package core

import "runtime"

// BinName is the file name of the core among the app's resources.
//
// On macOS it is not "mihomo". macOS 27 shows the kernel's socket list,
// which the core needs to tell which program opened a connection, only to
// a program whose code-signing identifier reads like a bundle identifier:
// one with a dot in it. A release build signs the core without naming an
// identifier, and codesign then makes one of the file's name less its last
// extension: "mihomo" for mihomo, which names no process and matches no
// rule for an application, and "dev.moho.app.core" for this name. (The
// core as upstream ships it is signed "a.out" by the linker, which passes,
// and an ad hoc build leaves that signature alone.)
var BinName = binName(runtime.GOOS)

func binName(goos string) string {
	switch goos {
	case "darwin":
		return "dev.moho.app.core.bin"
	case "windows":
		return "mihomo.exe"
	}
	return "mihomo"
}

// TunName is the file name of the copy of the core that TUN runs on, in
// the user's data directory. It is "mihomo" on macOS too: a copy keeps the
// signature it was made with whatever it is called, and the copies made
// by earlier versions stay where they are found.
func TunName(bin string) string {
	if runtime.GOOS == "darwin" {
		return "mihomo"
	}
	return bin
}
