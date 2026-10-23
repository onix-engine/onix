package resource

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestPackProgressCoversSortedInputsAndAliases(t *testing.T) {
	inputs := []PackInput{BytesInput("module/b.dat", []byte("same")), BytesInput("module/a.dat", []byte("same"))}
	var baseline, output bytes.Buffer
	if _, err := Pack(&baseline, inputs); err != nil {
		t.Fatal(err)
	}
	var events []PackProgress
	result, err := PackWithProgress(&output, inputs, func(p PackProgress) { events = append(events, p) })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), baseline.Bytes()) || result.Stats.Deduplicated != 1 {
		t.Fatal("progress changed package output or deduplication")
	}
	if len(events) != 8 || events[0].Path != "module/a.dat" || events[len(events)-1].Completed != 4 {
		t.Fatalf("missing inspection, write, or alias progress: %+v", events)
	}
	completed := 0
	for _, event := range events {
		if event.Total != 4 || event.Completed < completed || event.Completed > event.Total {
			t.Fatal("non-monotonic resource progress", event)
		}
		completed = event.Completed
	}
}

type failedPackageWriter struct{}

func (failedPackageWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestPackProgressDoesNotCompleteFailedWrite(t *testing.T) {
	var last PackProgress
	_, err := PackWithProgress(failedPackageWriter{}, []PackInput{BytesInput("module/value.dat", []byte("value"))}, func(p PackProgress) { last = p })
	if !errors.Is(err, io.ErrClosedPipe) || last.Completed == last.Total {
		t.Fatal("failed write reported completion", last, err)
	}
}
