package scene

import "testing"

func TestAnimationCurveSamplesLinearAndEaseIn(t *testing.T) {
	linear := LinearAnimationCurve()
	for _, progress := range []float32{0, .25, .5, .75, 1} {
		if got := linear.Sample(progress); got < progress-.001 || got > progress+.001 {
			t.Fatalf("linear curve at %v = %v", progress, got)
		}
	}
	easeIn := AnimationCurve{X1: .42, Y1: 0, X2: 1, Y2: 1}
	if got := easeIn.Sample(.5); got >= .5 {
		t.Fatalf("ease-in curve at .5 = %v, want less than .5", got)
	}
}

func TestAnimationCurveRejectsNonMonotonicTime(t *testing.T) {
	if err := (AnimationCurve{X1: -.1, X2: 1}).Validate(); err == nil {
		t.Fatal("expected invalid x control point")
	}
}
