package main

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	runLoopOnce sync.Once
	runLoopMode uintptr // kCFRunLoopDefaultMode
	runLoopRun  func(mode uintptr, seconds float64, returnAfterSource bool) int32
)

func runLoopLoad() {
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	mode, err := purego.Dlsym(cf, "kCFRunLoopDefaultMode")
	if err != nil {
		return
	}
	runLoopMode = **(**uintptr)(unsafe.Pointer(&mode)) // the symbol is a variable: read it
	purego.RegisterLibFunc(&runLoopRun, cf, "CFRunLoopRunInMode")
}

// pumpUntil keeps the main thread's run loop turning until done closes.
// Blocked instead, a window told to hide stays on the screen: AppKit only
// shows the change when the loop next turns. No clicks or keys are handled
// here. On the main thread.
func pumpUntil(done <-chan struct{}) {
	runLoopOnce.Do(runLoopLoad)
	if runLoopRun == nil {
		<-done
		return
	}
	for {
		select {
		case <-done:
			return
		default:
			runLoopRun(runLoopMode, 0.02, false)
		}
	}
}
