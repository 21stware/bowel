package tun

import (
	"errors"
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// A bowel that runs as root on Linux (in a container, or as a service of
// root's) starts the core as its child, and the child has every right
// TUN takes: there is nothing to install, and nothing to install it with
// where there is no shell, setcap or pkexec (a container's image is
// often bare). The core TUN runs on is then the bundled one. What can
// still be missing is the container's own: the device, and NET_ADMIN.

// What RootCore depends on. Tests replace them.
var (
	euid     = os.Geteuid
	rootGOOS = runtime.GOOS
	// tunDevice and procStatus are what DeviceReady looks at.
	tunDevice  = "/dev/net/tun"
	procStatus = "/proc/self/status"
)

// RootCore says whether the core bowel starts can make the TUN device as
// it is: bowel runs as root on Linux.
func RootCore() bool { return rootGOOS == "linux" && euid() == 0 }

// rootReady says whether path is a core that root can run as it is.
func rootReady(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}

// capNetAdmin is CAP_NET_ADMIN's number, the same on every architecture.
const capNetAdmin = 12

// DeviceReady says why a root bowel's core could not make the TUN device:
// no /dev/net/tun it may open, or no CAP_NET_ADMIN, which a container
// has only when it is given them. Without root it is nil: the
// installation gives the core its rights, and the core tells of the rest.
func DeviceReady() error {
	if !RootCore() {
		return nil
	}
	return deviceReady(tunDevice, procStatus)
}

func deviceReady(dev, status string) error {
	f, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errors.New(i18n.T("没有 /dev/net/tun，TUN 需要这个设备。在容器里运行时，请加上 --device /dev/net/tun 和 --cap-add NET_ADMIN",
				"There's no /dev/net/tun, which TUN needs. In a container, run it with --device /dev/net/tun and --cap-add NET_ADMIN"))
		}
		return errors.New(i18n.T("打不开 /dev/net/tun，TUN 需要这个设备。在容器里运行时，请加上 --device /dev/net/tun 和 --cap-add NET_ADMIN：",
			"Can't open /dev/net/tun, which TUN needs. In a container, run it with --device /dev/net/tun and --cap-add NET_ADMIN: ") + err.Error())
	}
	f.Close()
	b, err := os.ReadFile(status)
	if err != nil {
		return nil // not known: the core will tell
	}
	if have, known := effectiveCap(string(b), capNetAdmin); known && !have {
		return errors.New(i18n.T("bowel 以 root 运行，但没有 NET_ADMIN 权限，TUN 建不起来。在容器里运行时，请加上 --cap-add NET_ADMIN 和 --device /dev/net/tun",
			"bowel runs as root without NET_ADMIN, so TUN can't be made. In a container, run it with --cap-add NET_ADMIN and --device /dev/net/tun"))
	}
	return nil
}

// effectiveCap reads the CapEff line of a /proc/<pid>/status.
func effectiveCap(status string, c int) (have, known bool) {
	for _, line := range strings.Split(status, "\n") {
		v, ok := strings.CutPrefix(line, "CapEff:")
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(v), 16, 64)
		if err != nil {
			return false, false
		}
		return n&(1<<c) != 0, true
	}
	return false, false
}
