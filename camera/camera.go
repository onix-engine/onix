package camera

import (
	"errors"
	"image"
)

var (
	ErrUnavailable = errors.New("camera: camera capture is unavailable")
	ErrNotReady    = errors.New("camera: frame is not ready")
)

type Options struct {
	Width      int
	Height     int
	FacingMode string
}

type frameSource interface {
	Frame() (*image.NRGBA, error)
	Close() error
}

type Camera struct {
	source frameSource
}

func Available() bool {
	return platformAvailable()
}

func Open(options Options) (*Camera, error) {
	source, err := open(options)
	if err != nil {
		return nil, err
	}
	return &Camera{source: source}, nil
}

func (c *Camera) Frame() (*image.NRGBA, error) {
	if c == nil || c.source == nil {
		return nil, ErrUnavailable
	}
	return c.source.Frame()
}

func (c *Camera) Close() error {
	if c == nil || c.source == nil {
		return nil
	}
	err := c.source.Close()
	c.source = nil
	return err
}
