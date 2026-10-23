package style

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var errCSSColor = errors.New("invalid or unsupported CSS color")

// ParseColor parses an absolute CSS color. color-mix() currently supports the
// sRGB interpolation space; currentcolor requires an element's paint context.
func ParseColor(input string) (Color, error) { return parseColor(input) }

func isColorMix(input string) bool {
	return len(input) >= 10 && strings.EqualFold(input[:10], "color-mix(")
}

// Keep premultiplied floating-point components through nested mixes. Rounding
// each input to Color would accumulate errors, especially with low alpha.
func parseMixedColor(input string, depth int) (AnimationColor, error) {
	input = strings.TrimSpace(input)
	if depth >= 64 {
		return AnimationColor{}, errCSSColor
	}
	if strings.EqualFold(input, "currentcolor") {
		return AnimationColor{Current: 1}, nil
	}
	if !isColorMix(input) {
		if open := strings.IndexByte(input, '('); open >= 0 && (strings.EqualFold(input[:open], "rgb") || strings.EqualFold(input[:open], "rgba")) {
			r, g, b, a, err := parseRGBComponents(input)
			return AnimationColor{R: r * a, G: g * a, B: b * a, A: a}, err
		}
		color, err := parseColor(input)
		return ColorForAnimation(ColorValue(color)), err
	}
	if !strings.HasSuffix(input, ")") {
		return AnimationColor{}, errCSSColor
	}
	var parts [3]string
	n := 0
	for part := range cssValues(input[10:len(input)-1], false) {
		if n == len(parts) {
			return AnimationColor{}, errCSSColor
		}
		parts[n] = part
		n++
	}
	var method [2]string
	count, ok := cssFieldsInto(parts[0], method[:])
	if n != 3 || !ok || count != 2 || !strings.EqualFold(method[0], "in") || !strings.EqualFold(method[1], "srgb") {
		return AnimationColor{}, errCSSColor
	}
	left, p, err := parseColorMixStop(parts[1], depth+1)
	if err != nil {
		return AnimationColor{}, err
	}
	right, q, err := parseColorMixStop(parts[2], depth+1)
	if err != nil {
		return AnimationColor{}, err
	}
	if p < 0 && q < 0 {
		p, q = .5, .5
	} else if p < 0 {
		p = 1 - q
	} else if q < 0 {
		q = 1 - p
	}
	sum := p + q
	if sum == 0 {
		return AnimationColor{}, errCSSColor
	}
	// Totals below 100% reduce the resulting alpha. Scaling premultiplied
	// components as well preserves chroma when unpremultiplying at the end.
	scale := min(sum, 1) / sum
	p, q = p*scale, q*scale
	return AnimationColor{
		R: left.R*p + right.R*q, G: left.G*p + right.G*q,
		B: left.B*p + right.B*q, A: left.A*p + right.A*q,
		Current: left.Current*p + right.Current*q,
	}, nil
}

func parseColorMixStop(input string, depth int) (AnimationColor, float64, error) {
	var fields [2]string
	n, ok := cssFieldsInto(input, fields[:])
	if !ok || n == 0 {
		return AnimationColor{}, 0, errCSSColor
	}
	weight := -1.0
	color := fields[0]
	if n == 2 {
		percentage := fields[1]
		if strings.HasSuffix(color, "%") {
			color, percentage = percentage, color
		}
		if !strings.HasSuffix(percentage, "%") {
			return AnimationColor{}, 0, errCSSColor
		}
		value, err := parseColorNumber(percentage[:len(percentage)-1])
		if err != nil || value < 0 || value > 100 {
			return AnimationColor{}, 0, errCSSColor
		}
		weight = value / 100
	}
	parsed, err := parseMixedColor(color, depth)
	return parsed, weight, err
}

// parseRGBComponents supports legacy comma syntax and modern space/slash
// syntax without allocating or quantizing channels before interpolation.
func parseRGBComponents(input string) (r, g, b, a float64, err error) {
	a = 1
	open := strings.IndexByte(input, '(')
	if open < 0 || !strings.HasSuffix(input, ")") {
		return 0, 0, 0, 0, errCSSColor
	}
	body := input[open+1 : len(input)-1]
	var channels [4]string
	n := 0
	legacy := strings.Contains(body, ",")
	if legacy {
		for channel := range cssValues(body, false) {
			if n == len(channels) {
				return 0, 0, 0, 0, errCSSColor
			}
			channels[n] = channel
			n++
		}
	} else {
		rgb, alpha, slash := strings.Cut(body, "/")
		var ok bool
		n, ok = cssFieldsInto(rgb, channels[:3])
		if !ok || n != 3 {
			return 0, 0, 0, 0, errCSSColor
		}
		if slash {
			channels[3], n = strings.TrimSpace(alpha), 4
		}
	}
	if n != 3 && n != 4 {
		return 0, 0, 0, 0, errCSSColor
	}
	var values [4]float64
	for i := range n {
		channel := channels[i]
		percent := strings.HasSuffix(channel, "%")
		if legacy && i < 3 && percent != strings.HasSuffix(channels[0], "%") {
			return 0, 0, 0, 0, errCSSColor
		}
		if percent {
			channel = channel[:len(channel)-1]
		}
		number, parseErr := parseColorNumber(channel)
		if parseErr != nil {
			return 0, 0, 0, 0, parseErr
		}
		divisor := 1.0
		if percent {
			divisor = 100
		} else if i < 3 {
			divisor = 255
		}
		values[i] = min(max(number/divisor, 0), 1)
	}
	if n == 4 {
		a = values[3]
	}
	return values[0], values[1], values[2], a, nil
}

func parseColorNumber(input string) (float64, error) {
	// ParseFloat also accepts NaN, infinities, hexadecimal floats and digit
	// separators, none of which are CSS <number> tokens.
	i := 0
	if len(input) > 0 && (input[0] == '+' || input[0] == '-') {
		i++
	}
	start := i
	for i < len(input) && input[i] >= '0' && input[i] <= '9' {
		i++
	}
	digits := i - start
	if i < len(input) && input[i] == '.' {
		i++
		start = i
		for i < len(input) && input[i] >= '0' && input[i] <= '9' {
			i++
		}
		if i == start {
			return 0, errCSSColor
		}
		digits += i - start
	}
	if digits == 0 {
		return 0, errCSSColor
	}
	if i < len(input) && (input[i] == 'e' || input[i] == 'E') {
		i++
		if i < len(input) && (input[i] == '+' || input[i] == '-') {
			i++
		}
		start = i
		for i < len(input) && input[i] >= '0' && input[i] <= '9' {
			i++
		}
		if i == start {
			return 0, errCSSColor
		}
	}
	if i != len(input) {
		return 0, errCSSColor
	}
	value, err := strconv.ParseFloat(input, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, errCSSColor
	}
	return value, nil
}
