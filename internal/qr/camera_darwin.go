package qr

// The camera, through AVFoundation, called with purego (no cgo): an
// AVCaptureSession whose video data output hands BGRA frames to a delegate
// class registered once. One camera is open at a time.

import (
	"image"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	camOnce sync.Once
	camErr  error

	camDelegate   objc.ID
	camQueueLabel = []byte("dev.moho.camera\x00") // kept: the queue may hold it
	camQueue      uintptr
	camCurrent    atomic.Pointer[Camera]

	dispatchQueueCreate           func(label *byte, attr uintptr) uintptr
	cmSampleBufferGetImageBuffer  func(buf uintptr) uintptr
	cvPixelBufferLockBaseAddress  func(pb uintptr, flags uint64) int32
	cvPixelBufferUnlockBaseAddres func(pb uintptr, flags uint64) int32
	cvPixelBufferGetBaseAddress   func(pb uintptr) uintptr
	cvPixelBufferGetBytesPerRow   func(pb uintptr) uintptr
	cvPixelBufferGetWidth         func(pb uintptr) uintptr
	cvPixelBufferGetHeight        func(pb uintptr) uintptr
	cvPixelFormatKey              uintptr // kCVPixelBufferPixelFormatTypeKey
	avMediaTypeVideo              uintptr // AVMediaTypeVideo
)

func sel(s string) objc.SEL     { return objc.RegisterName(s) }
func class(s string) objc.ID    { return objc.ID(objc.GetClass(s)) }
func deref(p uintptr) uintptr   { return *(*uintptr)(unsafe.Add(nil, p)) } // C memory
func nsString(s string) objc.ID { return class("NSString").Send(sel("stringWithUTF8String:"), s) }
func camSetup() error           { camOnce.Do(func() { camErr = camLoad() }); return camErr }
func lib(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

func camLoad() error {
	sys, err := lib("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return err
	}
	av, err := lib("/System/Library/Frameworks/AVFoundation.framework/AVFoundation")
	if err != nil {
		return err
	}
	cm, err := lib("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
	if err != nil {
		return err
	}
	cv, err := lib("/System/Library/Frameworks/CoreVideo.framework/CoreVideo")
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&dispatchQueueCreate, sys, "dispatch_queue_create")
	purego.RegisterLibFunc(&cmSampleBufferGetImageBuffer, cm, "CMSampleBufferGetImageBuffer")
	purego.RegisterLibFunc(&cvPixelBufferLockBaseAddress, cv, "CVPixelBufferLockBaseAddress")
	purego.RegisterLibFunc(&cvPixelBufferUnlockBaseAddres, cv, "CVPixelBufferUnlockBaseAddress")
	purego.RegisterLibFunc(&cvPixelBufferGetBaseAddress, cv, "CVPixelBufferGetBaseAddress")
	purego.RegisterLibFunc(&cvPixelBufferGetBytesPerRow, cv, "CVPixelBufferGetBytesPerRow")
	purego.RegisterLibFunc(&cvPixelBufferGetWidth, cv, "CVPixelBufferGetWidth")
	purego.RegisterLibFunc(&cvPixelBufferGetHeight, cv, "CVPixelBufferGetHeight")
	sym, err := purego.Dlsym(cv, "kCVPixelBufferPixelFormatTypeKey")
	if err != nil {
		return err
	}
	cvPixelFormatKey = deref(sym)
	if sym, err = purego.Dlsym(av, "AVMediaTypeVideo"); err != nil {
		return err
	}
	avMediaTypeVideo = deref(sym)

	cls, err := objc.RegisterClass("MohoCameraDelegate", objc.GetClass("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("AVCaptureVideoDataOutputSampleBufferDelegate")}, nil,
		[]objc.MethodDef{{
			Cmd: sel("captureOutput:didOutputSampleBuffer:fromConnection:"),
			Fn: func(self objc.ID, _ objc.SEL, output, buf, conn objc.ID) {
				if c := camCurrent.Load(); c != nil {
					c.frame(uintptr(buf))
				}
			},
		}})
	if err != nil {
		return err
	}
	camDelegate = objc.ID(cls).Send(sel("alloc")).Send(sel("init"))
	camQueue = dispatchQueueCreate(&camQueueLabel[0], 0)
	return nil
}

// ErrNoCamera means the Mac has no camera, or none it lets Moho open.
var ErrNoCamera error = &msg{"没有可用的摄像头", "No camera available"}

// Camera is an open camera. Frames come at most every 1/15 s; a frame the
// reader has not taken yet is dropped for the next.
type Camera struct {
	session, output objc.ID
	frames          chan *image.RGBA
	last            time.Time
	once            sync.Once
	mu              sync.Mutex
	closed          bool
}

// OpenCamera starts the default camera. Ask for the permission first: a
// Mac that has not been asked shows its prompt and gives no frames until
// the user answers.
func OpenCamera() (*Camera, error) {
	if err := camSetup(); err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := class("NSAutoreleasePool").Send(sel("alloc")).Send(sel("init"))
	defer pool.Send(sel("drain"))

	dev := class("AVCaptureDevice").Send(sel("defaultDeviceWithMediaType:"), avMediaTypeVideo)
	if dev == 0 {
		return nil, ErrNoCamera
	}
	var nserr objc.ID
	in := class("AVCaptureDeviceInput").Send(sel("deviceInputWithDevice:error:"), dev, unsafe.Pointer(&nserr))
	if in == 0 {
		return nil, ErrNoCamera
	}
	c := &Camera{frames: make(chan *image.RGBA, 1)}
	c.session = class("AVCaptureSession").Send(sel("alloc")).Send(sel("init"))
	c.session.Send(sel("beginConfiguration"))
	if preset := nsString("AVCaptureSessionPreset1280x720"); objc.Send[bool](c.session, sel("canSetSessionPreset:"), preset) {
		c.session.Send(sel("setSessionPreset:"), preset)
	}
	if !objc.Send[bool](c.session, sel("canAddInput:"), in) {
		c.session.Send(sel("release"))
		return nil, ErrNoCamera
	}
	c.session.Send(sel("addInput:"), in)
	c.output = class("AVCaptureVideoDataOutput").Send(sel("alloc")).Send(sel("init"))
	c.output.Send(sel("setAlwaysDiscardsLateVideoFrames:"), true)
	bgra := class("NSNumber").Send(sel("numberWithUnsignedInt:"), uint32(0x42475241)) // 'BGRA'
	c.output.Send(sel("setVideoSettings:"), class("NSDictionary").Send(sel("dictionaryWithObject:forKey:"), bgra, cvPixelFormatKey))
	c.output.Send(sel("setSampleBufferDelegate:queue:"), camDelegate, camQueue)
	if !objc.Send[bool](c.session, sel("canAddOutput:"), c.output) {
		c.output.Send(sel("release"))
		c.session.Send(sel("release"))
		return nil, ErrNoCamera
	}
	c.session.Send(sel("addOutput:"), c.output)
	c.session.Send(sel("commitConfiguration"))
	if old := camCurrent.Swap(c); old != nil {
		old.Close()
	}
	c.session.Send(sel("startRunning"))
	return c, nil
}

// Frames gives the camera's frames, upright as the camera sees them (not
// mirrored). It closes with the camera.
func (c *Camera) Frames() <-chan *image.RGBA { return c.frames }

// frame copies a sample buffer's pixels, on the capture queue.
func (c *Camera) frame(buf uintptr) {
	now := time.Now()
	if now.Sub(c.last) < time.Second/15 || len(c.frames) > 0 {
		return
	}
	c.last = now
	pb := cmSampleBufferGetImageBuffer(buf)
	if pb == 0 {
		return
	}
	cvPixelBufferLockBaseAddress(pb, 1) // read only
	base := cvPixelBufferGetBaseAddress(pb)
	stride := int(cvPixelBufferGetBytesPerRow(pb))
	w, h := int(cvPixelBufferGetWidth(pb)), int(cvPixelBufferGetHeight(pb))
	var img *image.RGBA
	if base != 0 && w > 0 && h > 0 && stride >= 4*w {
		src := unsafe.Slice((*byte)(unsafe.Add(nil, base)), stride*h)
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		for y := range h {
			s, d := src[y*stride:y*stride+4*w], img.Pix[y*img.Stride:y*img.Stride+4*w]
			for x := 0; x < 4*w; x += 4 {
				d[x], d[x+1], d[x+2], d[x+3] = s[x+2], s[x+1], s[x], 0xff
			}
		}
	}
	cvPixelBufferUnlockBaseAddres(pb, 1)
	if img == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.frames <- img:
	default:
	}
}

// Close stops the camera, and its light.
func (c *Camera) Close() {
	c.once.Do(func() {
		camCurrent.CompareAndSwap(c, nil)
		c.session.Send(sel("stopRunning"))
		c.output.Send(sel("setSampleBufferDelegate:queue:"), objc.ID(0), uintptr(0))
		c.session.Send(sel("release"))
		c.output.Send(sel("release"))
		c.mu.Lock()
		c.closed = true
		close(c.frames)
		c.mu.Unlock()
	})
}
