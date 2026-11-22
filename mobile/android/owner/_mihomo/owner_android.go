//go:build android

// This file is mihomo's, not the owner module's: mobile/android/build.sh
// copies it into the core's main package, where it lets a rule for an app
// (PROCESS-NAME, with the app's package name) match. The go tool skips a
// folder whose name starts with an underscore, so it is built only there.

package main

import (
	"os"

	"github.com/metacubex/mihomo/component/process"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"github.com/oboo/moho/mobile/android/owner"
)

// This init runs after those of the packages main imports, so it replaces
// the resolver mihomo's TUN sets on Android, which reads a file apps may not
// open.
func init() {
	path := os.Getenv(owner.SocketEnv)
	if path == "" {
		return
	}
	s, err := owner.Listen(path)
	if err != nil {
		log.Warnln("[Process] apps can't be told apart: %v", err)
		return
	}
	process.DefaultPackageNameResolver = func(m *C.Metadata) (string, error) {
		src := owner.AddrPort(m.RawSrcAddr, m.SrcIP, m.SrcPort)
		dst := owner.AddrPort(m.RawDstAddr, m.DstIP, m.DstPort)
		uid, pkg, err := s.Lookup(m.NetWork.String(), src, dst)
		if err != nil {
			return "", err
		}
		m.Uid = uid
		if pkg == "" {
			return "", process.ErrNotFound
		}
		return pkg, nil
	}
}
