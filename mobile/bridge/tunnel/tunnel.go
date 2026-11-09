// Package tunnel runs mihomo as a library inside the iOS packet tunnel
// extension. It reads the config.yaml the app wrote into the App Group
// folder, points the core's TUN at the utun descriptor the system gave the
// extension and serves the controller on a unix socket in the same folder.
//
// It is a package of its own so the extension links mihomo and the app does
// not: gomobile builds one framework for each.
package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	"gopkg.in/yaml.v3"
)

var (
	mu      sync.Mutex
	running bool
)

// SocketName is the controller's socket, in the App Group folder.
const SocketName = "c.sock"

const tunFDFile = "tun.fd"

// Start blocks until the core is up, or returns why it could not start.
// memLimitMB bounds the Go heap softly; 0 leaves it alone.
func Start(dir string, fd int32, memLimitMB int32) error {
	mu.Lock()
	defer mu.Unlock()
	if running {
		return fmt.Errorf("already running")
	}
	if memLimitMB > 0 {
		debug.SetMemoryLimit(int64(memLimitMB) << 20)
	}
	debug.SetGCPercent(50)

	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	tun, _ := cfg["tun"].(map[string]any)
	if tun == nil {
		return fmt.Errorf("config has no tun section")
	}
	tun["file-descriptor"] = int(fd)
	// The app names the same descriptor when it reloads the config.
	if err := os.WriteFile(filepath.Join(dir, tunFDFile), []byte(strconv.Itoa(int(fd))), 0o600); err != nil {
		return err
	}
	patched, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	C.SetHomeDir(dir)
	sock := filepath.Join(dir, SocketName)
	_ = os.Remove(sock)
	if err := hub.Parse(patched, hub.WithExternalControllerUnix(sock)); err != nil {
		return err
	}
	running = true
	return nil
}

// Stop shuts the core down.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if !running {
		return
	}
	executor.Shutdown()
	running = false
}

// FreeMemory returns what the Go runtime can give back to the system. The
// extension calls it when the system warns about memory.
func FreeMemory() { debug.FreeOSMemory() }

// GoStats describes the Go runtime's memory, as "heapInuse sys goroutines"
// in KiB, KiB and a count.
func GoStats() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("%d %d %d %d", m.HeapInuse>>10, m.Sys>>10, runtime.NumGoroutine(), time.Now().UnixMilli())
}
