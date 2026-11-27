//go:build cgo && (windows || (linux && !android && !onix_drm))

package window

func (l *EventLoop) runFrameBatch() error { return l.dispatchFrames() }
