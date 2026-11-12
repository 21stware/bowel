package procinfo

import (
	"os"
	"strconv"
	"syscall"
)

// running reads /proc: the processes this user owns, by their executables.
// A kernel thread has none.
func running() []string {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	uid := uint32(os.Getuid())
	var out []string
	for _, e := range ents {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		dir := "/proc/" + e.Name()
		if fi, err := os.Stat(dir); err != nil || fi.Sys().(*syscall.Stat_t).Uid != uid {
			continue
		}
		if exe, err := os.Readlink(dir + "/exe"); err == nil && exe != "" {
			out = append(out, exe)
		}
	}
	return out
}
