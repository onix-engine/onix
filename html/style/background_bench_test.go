package style

import "testing"

func BenchmarkBackgroundShorthand(b *testing.B) {
	for _, test := range []struct{ name, value string }{
		{"color", "red"},
		{"single", "url(card.png) 10px 20px / 30px 40px no-repeat"},
		{"multiple", "url(front.png) left top no-repeat, url(back.png) right bottom repeat-x"},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				computed := defaultStyle(nil)
				if err := applyProperty(&computed, "background", test.value, nil, parseContext{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
