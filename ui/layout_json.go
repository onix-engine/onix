//go:build !onix_release

package ui

import "encoding/json"

func (w *Widget) UnmarshalJSON(data []byte) error {
	type widgetAlias Widget
	var decoded struct {
		*widgetAlias
		Enabled *bool `json:"enabled"`
	}
	decoded.widgetAlias = (*widgetAlias)(w)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Enabled == nil {
		w.Enabled = true
	}
	return nil
}
