//go:build !windows

package tun

// AsksEachStart is false: the authorization is asked once, by Install, and
// the copy it makes has its rights by itself.
const AsksEachStart = false

// Task is the core run some other way than as a child of the app. Only
// Windows has one: elsewhere the installed copy has its rights by itself,
// and runs as any child does.
type Task struct{}

// Spawn starts nothing here, and says so with a nil task.
func Spawn(bin, src string) (*Task, error) { return nil, nil }

func (*Task) Pid() int    { return 0 }
func (*Task) Wait() error { return nil }
func (*Task) Stop()       {}
