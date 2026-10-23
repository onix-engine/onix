package spine

import "testing"

func TestPlayerAnimationPlaybackState(t *testing.T) {
	resource := &Resource{animations: map[string]animation{
		"idle":   {duration: 2},
		"attack": {duration: .75},
	}}
	player := NewPlayer(resource)
	if err := player.SetAnimation("attack", false); err != nil {
		t.Fatal(err)
	}
	if duration := player.AnimationDuration(); duration != .75 {
		t.Fatalf("animation duration = %v, want .75", duration)
	}
	player.Update(.5)
	if player.AnimationComplete() {
		t.Fatal("animation completed before its duration")
	}
	if elapsed := player.AnimationTime(); elapsed != .5 {
		t.Fatalf("animation time = %v, want .5", elapsed)
	}
	player.Update(.25)
	if !player.AnimationComplete() {
		t.Fatal("animation did not complete at its duration")
	}

	if err := player.SetAnimation("idle", true); err != nil {
		t.Fatal(err)
	}
	player.Update(3)
	if player.AnimationComplete() {
		t.Fatal("looping animation reported completion")
	}
}

func TestNilPlayerAnimationPlaybackState(t *testing.T) {
	var player *Player
	if player.AnimationDuration() != 0 || player.AnimationTime() != 0 || player.AnimationComplete() {
		t.Fatal("nil player reported animation playback state")
	}
}
