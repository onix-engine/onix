package webview

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestWebViewBridgeResponseIsValidJSON(t *testing.T) {
	for _, test := range []struct {
		name   string
		result string
		err    error
	}{
		{name: "result", result: "hello\n\x01世界"},
		{name: "error", err: errors.New("bad \"request\"")},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded := appendWebViewBridgeResponse(nil, 7, test.result, test.err)
			if !json.Valid(encoded) {
				t.Fatalf("invalid JSON response: %q", encoded)
			}
			var response struct {
				ID     uint64 `json:"id"`
				OK     bool   `json:"ok"`
				Result string `json:"result"`
				Error  string `json:"error"`
			}
			if err := json.Unmarshal(encoded, &response); err != nil {
				t.Fatal(err)
			}
			if response.ID != 7 || response.OK != (test.err == nil) || response.Result != test.result {
				t.Fatalf("unexpected response: %+v", response)
			}
			if test.err != nil && response.Error != test.err.Error() {
				t.Fatalf("error = %q, want %q", response.Error, test.err)
			}
		})
	}
}

func TestDecodeWebViewBridgeRequest(t *testing.T) {
	var request webViewBridgeRequest
	if err := decodeWebViewBridgeRequest(`{"id":9,"name":"ping","payload":"{\"x\":1}"}`, &request); err != nil {
		t.Fatal(err)
	}
	if request.ID != 9 || request.Name != "ping" || request.Payload != `{"x":1}` {
		t.Fatalf("request = %+v", request)
	}
}
