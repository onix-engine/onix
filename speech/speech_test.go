package speech

import "testing"

func TestSystemSpeechUnavailablePlatform(t *testing.T) {
	synthesizer := New()
	defer synthesizer.Close()
	if synthesizer.Available() {
		t.Skip("system speech is available on this platform")
	}
	if err := synthesizer.Speak("hello", Options{}); err != ErrUnavailable {
		t.Fatalf("Speak error = %v, want %v", err, ErrUnavailable)
	}
	if status := synthesizer.Status(); status.Err != ErrUnavailable {
		t.Fatalf("Status = %+v", status)
	}
}
