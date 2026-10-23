package webview

import (
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"
)

const windowsWebViewBridgeScript = `(function(){
  "use strict";
  var pending = new Map(), nextID = 0;
  function invoke(name, payload) {
    return new Promise(function(resolve, reject) {
      var id = ++nextID;
      pending.set(id, {resolve: resolve, reject: reject});
      window.chrome.webview.postMessage(JSON.stringify({
        id: id,
        name: String(name),
        payload: String(payload == null ? "" : payload)
      }));
    });
  }
  window.chrome.webview.addEventListener("message", function(event) {
    if (typeof event.data !== "string") return;
    var response;
    try { response = JSON.parse(event.data); } catch (_) { return; }
    var callbacks = pending.get(response.id);
    if (!callbacks) return;
    pending.delete(response.id);
    if (response.ok) callbacks.resolve(response.result);
    else callbacks.reject(new Error(response.error || "webview invocation failed"));
  });
  globalThis.__onixInvoke = invoke;
  globalThis.onix = globalThis.onix || {};
  globalThis.onix.invoke = invoke;
})();`

type webViewBridgeRequest struct {
	ID      uint64 `json:"id"`
	Name    string `json:"name"`
	Payload string `json:"payload"`
}

func decodeWebViewBridgeRequest(message string, request *webViewBridgeRequest) error {
	if request == nil {
		return errors.New("window: WebView2 bridge request is nil")
	}
	*request = webViewBridgeRequest{}
	if err := json.Unmarshal([]byte(message), request); err != nil {
		return err
	}
	if request.ID == 0 {
		return errors.New("window: WebView2 bridge request has no id")
	}
	return nil
}

func appendWebViewBridgeResponse(buffer []byte, id uint64, result string, responseErr error) []byte {
	buffer = append(buffer[:0], `{"id":`...)
	buffer = strconv.AppendUint(buffer, id, 10)
	if responseErr == nil {
		buffer = append(buffer, `,"ok":true,"result":`...)
		buffer = appendJSONString(buffer, result)
	} else {
		buffer = append(buffer, `,"ok":false,"error":`...)
		buffer = appendJSONString(buffer, responseErr.Error())
	}
	return append(buffer, '}')
}

func appendJSONString(buffer []byte, value string) []byte {
	const hex = "0123456789abcdef"
	buffer = append(buffer, '"')
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		value = value[size:]
		switch r {
		case '"', '\\':
			buffer = append(buffer, '\\', byte(r))
		case '\b':
			buffer = append(buffer, `\b`...)
		case '\f':
			buffer = append(buffer, `\f`...)
		case '\n':
			buffer = append(buffer, `\n`...)
		case '\r':
			buffer = append(buffer, `\r`...)
		case '\t':
			buffer = append(buffer, `\t`...)
		default:
			if r < 0x20 {
				buffer = append(buffer, '\\', 'u', '0', '0', hex[byte(r)>>4], hex[byte(r)&0xf])
			} else {
				buffer = utf8.AppendRune(buffer, r)
			}
		}
	}
	return append(buffer, '"')
}
