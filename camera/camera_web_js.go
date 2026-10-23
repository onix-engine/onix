//go:build !wechat

package camera

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"syscall/js"
)

type webCamera struct {
	stream js.Value
	video  js.Value
	canvas js.Value
	ctx    js.Value
	width  int
	height int
	closed bool
}

type promiseResult struct {
	value js.Value
	err   error
}

func platformAvailable() bool {
	navigator := js.Global().Get("navigator")
	return navigator.Truthy() && navigator.Get("mediaDevices").Truthy() &&
		navigator.Get("mediaDevices").Get("getUserMedia").Type() == js.TypeFunction
}

func open(options Options) (frameSource, error) {
	if !platformAvailable() {
		return nil, ErrUnavailable
	}
	if options.Width <= 0 {
		options.Width = 960
	}
	if options.Height <= 0 {
		options.Height = 720
	}
	if strings.TrimSpace(options.FacingMode) == "" {
		options.FacingMode = "environment"
	}
	videoOptions := js.Global().Get("Object").New()
	videoOptions.Set("width", idealConstraint(options.Width))
	videoOptions.Set("height", idealConstraint(options.Height))
	videoOptions.Set("facingMode", idealConstraint(options.FacingMode))
	constraints := js.Global().Get("Object").New()
	constraints.Set("audio", false)
	constraints.Set("video", videoOptions)
	stream, err := awaitPromise(js.Global().Get("navigator").Get("mediaDevices").Call("getUserMedia", constraints))
	if err != nil {
		return nil, fmt.Errorf("camera: open video stream: %w", err)
	}
	document := js.Global().Get("document")
	video := document.Call("createElement", "video")
	video.Set("autoplay", true)
	video.Set("muted", true)
	video.Set("playsInline", true)
	video.Set("srcObject", stream)
	if _, err := awaitPromise(video.Call("play")); err != nil {
		stopStream(stream)
		return nil, fmt.Errorf("camera: start video stream: %w", err)
	}
	canvas := document.Call("createElement", "canvas")
	contextOptions := js.Global().Get("Object").New()
	contextOptions.Set("willReadFrequently", true)
	context := canvas.Call("getContext", "2d", contextOptions)
	if !context.Truthy() {
		stopStream(stream)
		return nil, errors.New("camera: 2D canvas is unavailable")
	}
	return &webCamera{
		stream: stream, video: video, canvas: canvas, ctx: context,
		width: options.Width, height: options.Height,
	}, nil
}

func (c *webCamera) Frame() (frame *image.NRGBA, err error) {
	if c == nil || c.closed {
		return nil, ErrUnavailable
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			frame = nil
			err = fmt.Errorf("camera: capture frame: %v", recovered)
		}
	}()
	sourceWidth, sourceHeight := c.video.Get("videoWidth").Int(), c.video.Get("videoHeight").Int()
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return nil, ErrNotReady
	}
	width, height := fitSize(sourceWidth, sourceHeight, c.width, c.height)
	if c.canvas.Get("width").Int() != width || c.canvas.Get("height").Int() != height {
		c.canvas.Set("width", width)
		c.canvas.Set("height", height)
	}
	c.ctx.Call("drawImage", c.video, 0, 0, width, height)
	pixels := c.ctx.Call("getImageData", 0, 0, width, height).Get("data")
	frame = image.NewNRGBA(image.Rect(0, 0, width, height))
	if copied := js.CopyBytesToGo(frame.Pix, pixels); copied != len(frame.Pix) {
		return nil, fmt.Errorf("camera: copied %d of %d frame bytes", copied, len(frame.Pix))
	}
	return frame, nil
}

func (c *webCamera) Close() error {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	c.video.Call("pause")
	c.video.Set("srcObject", js.Null())
	stopStream(c.stream)
	return nil
}

func idealConstraint(value any) js.Value {
	constraint := js.Global().Get("Object").New()
	constraint.Set("ideal", value)
	return constraint
}

func awaitPromise(promise js.Value) (js.Value, error) {
	done := make(chan promiseResult, 1)
	resolve := js.FuncOf(func(this js.Value, args []js.Value) any {
		var value js.Value
		if len(args) > 0 {
			value = args[0]
		}
		done <- promiseResult{value: value}
		return nil
	})
	reject := js.FuncOf(func(this js.Value, args []js.Value) any {
		var value js.Value
		if len(args) > 0 {
			value = args[0]
		}
		done <- promiseResult{err: jsError(value)}
		return nil
	})
	promise.Call("then", resolve).Call("catch", reject)
	result := <-done
	resolve.Release()
	reject.Release()
	return result.value, result.err
}

func jsError(value js.Value) error {
	if !value.Truthy() {
		return errors.New("browser request failed")
	}
	name, message := value.Get("name").String(), value.Get("message").String()
	if name != "" && message != "" {
		return errors.New(name + ": " + message)
	}
	if message != "" {
		return errors.New(message)
	}
	return errors.New(value.String())
}

func stopStream(stream js.Value) {
	if !stream.Truthy() {
		return
	}
	tracks := stream.Call("getTracks")
	for index := 0; index < tracks.Length(); index++ {
		tracks.Index(index).Call("stop")
	}
}

func fitSize(sourceWidth, sourceHeight, maximumWidth, maximumHeight int) (int, int) {
	if sourceWidth <= maximumWidth && sourceHeight <= maximumHeight {
		return sourceWidth, sourceHeight
	}
	if sourceWidth*maximumHeight > sourceHeight*maximumWidth {
		return maximumWidth, max(1, sourceHeight*maximumWidth/sourceWidth)
	}
	return max(1, sourceWidth*maximumHeight/sourceHeight), maximumHeight
}
