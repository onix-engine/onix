//go:build onix_mobile_test

package speech

import "testing"

func TestMobileSpeechBridgeLifecycle(t *testing.T) {
	mobileState.Lock()
	mobileState.sequence = 0
	mobileState.command = 0
	mobileState.text = ""
	mobileState.options = Options{}
	mobileState.available = false
	mobileState.status = Status{}
	mobileState.Unlock()
	DispatchNativeEvent(NativeEventAvailable)
	synthesizer := New()
	defer synthesizer.Close()
	if !synthesizer.Available() {
		t.Fatal("mobile system speech is unavailable after native event")
	}
	options := Options{Language: "zh-CN", Voice: "local", Rate: 1.25, Pitch: 1.1, Volume: 0.8}
	if err := synthesizer.Speak("hello", options); err != nil {
		t.Fatal(err)
	}
	if NativeCommand(NativeCommandValue()&0xff) != NativeCommandSpeak || NativeText() != "hello" || NativeOptions() != options {
		t.Fatalf("native speech command=%d text=%q options=%+v", NativeCommandValue()&0xff, NativeText(), NativeOptions())
	}
	DispatchNativeEvent(NativeEventStarted)
	if status := synthesizer.Status(); !status.Speaking || status.Paused {
		t.Fatalf("started status = %+v", status)
	}
	if err := synthesizer.Pause(); err != nil {
		t.Fatal(err)
	}
	if NativeCommand(NativeCommandValue()&0xff) != NativeCommandPause || !synthesizer.Status().Paused {
		t.Fatalf("paused status = %+v", synthesizer.Status())
	}
	if err := synthesizer.Resume(); err != nil {
		t.Fatal(err)
	}
	if NativeCommand(NativeCommandValue()&0xff) != NativeCommandResume || synthesizer.Status().Paused {
		t.Fatalf("resumed status = %+v", synthesizer.Status())
	}
	DispatchNativeEvent(NativeEventCompleted)
	if status := synthesizer.Status(); status.Speaking || status.Err != nil {
		t.Fatalf("completed status = %+v", status)
	}
}
