//go:build !darwin

package qr

import (
	"errors"
	"image"
)

var ErrNoCamera error = &msg{"没有可用的摄像头", "No camera available"}

type Camera struct{}

func OpenCamera() (*Camera, error)           { return nil, errors.ErrUnsupported }
func (c *Camera) Frames() <-chan *image.RGBA { return nil }
func (c *Camera) Close()                     {}
