package browser

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/diiyw/onix/examples/webview/contracts"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
)

type Mod struct{ bounds module.Slot }

func (m *Mod) Install(c *module.Context) error {
	file, err := c.OpenResource("page")
	if err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	page := scene.NewWebViewHTML(string(data))
	page.Bind("ping", func(payload string) (string, error) {
		if !c.Active() {
			return "", module.ErrUnavailable
		}
		if !json.Valid([]byte(payload)) {
			return "", fmt.Errorf("ping payload must be valid JSON")
		}
		result, err := json.Marshal(struct {
			Message string          `json:"message"`
			Payload json.RawMessage `json:"payload"`
		}{"hello from Go", json.RawMessage(payload)})
		return string(result), err
	})
	if err := c.Set(c.Root(), page); err != nil {
		return err
	}
	if m.bounds, err = c.Slot("bounds"); err != nil {
		return err
	}
	if err := module.OnSlotsChange(c, []string{"bounds"}, m.resize); err != nil {
		return err
	}
	return m.resize(c)
}

func (m *Mod) resize(c *module.Context) error {
	value, err := m.bounds.Read[contracts.BrowserBounds]()
	if err != nil {
		return err
	}
	if value.Width <= 0 || value.Height <= 0 {
		return module.ErrInvalid
	}
	page, ok := c.Get[scene.WebView](c.Root())
	if !ok {
		return module.ErrType
	}
	page.Bounds = scene.WebViewBounds{X: float32(value.X), Y: float32(value.Y), Width: float32(value.Width), Height: float32(value.Height)}
	return c.Set(c.Root(), page)
}
