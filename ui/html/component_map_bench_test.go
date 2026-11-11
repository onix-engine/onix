package html

import "testing"

var componentMapStateSink htmlComponentVNodeState

func BenchmarkHTMLComponentStateCreate(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		componentMapStateSink = newHTMLComponentVNodeState()
	}
}
