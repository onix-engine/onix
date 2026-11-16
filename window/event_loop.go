//go:build cgo && (windows || (darwin && !ios) || (linux && !android && !onix_drm))

package window

import "time"

type loopWindow struct {
	context *Context
	frame   func() (bool, error)
	closed  func()
}

// EventLoop runs native windows on one OS thread. Every window retains its own
// rendering context, input queue and idle deadline. Add windows between frames.
type EventLoop struct {
	BetweenFrames func() error
	windows       []loopWindow
	waiter        eventWaiter
}

func (l *EventLoop) Add(context *Context, frame func() (bool, error), closed func()) {
	l.windows = append(l.windows, loopWindow{context: context, frame: frame, closed: closed})
}

func (l *EventLoop) close(index int) {
	w := l.windows[index]
	copy(l.windows[index:], l.windows[index+1:])
	l.windows[len(l.windows)-1] = loopWindow{}
	l.windows = l.windows[:len(l.windows)-1]
	if w.closed != nil {
		w.context.ActivateRender()
		w.closed()
	}
}

// Close releases every remaining window, including after an update error.
func (l *EventLoop) Close() {
	for len(l.windows) != 0 {
		l.close(len(l.windows) - 1)
	}
}

func (l *EventLoop) Run() error {
	defer l.Close()
	for len(l.windows) != 0 {
		for i := 0; i < len(l.windows); {
			w := &l.windows[i]
			c := w.context
			c.ActivateRender()
			if err := c.pollFrame(); err != nil {
				return err
			}
			if c.shouldClose {
				l.close(i)
				continue
			}
			if c.frameVisible() {
				wait, idle := c.idleWait()
				if (!idle || wait == 0) && c.framePacer.ready(time.Now()) {
					rendered, err := w.frame()
					if err != nil {
						return err
					}
					if rendered {
						if err := c.presentFrame(); err != nil {
							return err
						}
					}
				}
			}
			if c.shouldClose {
				l.close(i)
				continue
			}
			i++
		}
		if l.BetweenFrames != nil {
			if err := l.BetweenFrames(); err != nil {
				return err
			}
		}
		if len(l.windows) != 0 {
			l.waiter.wait(l.windows, l.waitDuration())
		}
	}
	return nil
}

func (l *EventLoop) waitDuration() time.Duration {
	wait := time.Duration(-1)
	now := time.Now()
	for _, w := range l.windows {
		c := w.context
		if c.shouldClose {
			return 0
		}
		if !c.frameVisible() {
			continue
		}
		next, idle := c.idleWait()
		if !idle || next == 0 {
			next = max(0, c.framePacer.nextFrame.Sub(now))
		}
		if next >= 0 && (wait < 0 || next < wait) {
			wait = next
		}
	}
	return wait
}
