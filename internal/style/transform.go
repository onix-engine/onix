package style

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type transformValue struct {
	Rotation                         float64
	ScaleX, ScaleY                   float64
	SkewX                            float64
	TranslateCrossX, TranslateCrossY float64
	TranslateX                       Value
	TranslateY                       Value
}

func parseTransform(input string, ctx parseContext) (transformValue, error) {
	trimmed := strings.TrimSpace(input)
	result := transformValue{TranslateX: LengthPx(0), TranslateY: LengthPx(0), ScaleX: 1, ScaleY: 1}
	if strings.EqualFold(trimmed, "none") {
		return result, nil
	}
	if trimmed == "" {
		return result, fmt.Errorf("unsupported transform %q", input)
	}
	a, b, c, d := float64(1), float64(0), float64(0), float64(1)
	decompose := false
	for offset := 0; offset < len(trimmed); {
		for offset < len(trimmed) && isCSSWhitespace(trimmed[offset]) {
			offset++
		}
		if offset == len(trimmed) {
			break
		}
		open := strings.IndexByte(trimmed[offset:], '(')
		if open < 0 {
			return result, fmt.Errorf("unsupported transform %q", input)
		}
		open += offset
		name := strings.TrimSpace(trimmed[offset:open])
		close := transformFunctionEnd(trimmed, open)
		if close < 0 {
			return result, fmt.Errorf("unsupported transform %q", input)
		}
		args := trimmed[open+1 : close]
		switch {
		case strings.EqualFold(name, "rotate"):
			rotation, err := parseAngleDegrees(strings.TrimSpace(args))
			if err != nil {
				return result, err
			}
			decompose = decompose || result.ScaleX != result.ScaleY
			sn, cs := math.Sincos(rotation * math.Pi / 180)
			a, b, c, d = a*cs+c*sn, b*cs+d*sn, c*cs-a*sn, d*cs-b*sn
			if math.IsNaN(rotation) || math.IsInf(rotation, 0) {
				return result, fmt.Errorf("invalid rotation %q", args)
			}
			result.Rotation += rotation
		case strings.EqualFold(name, "translatex"), strings.EqualFold(name, "translatey"), strings.EqualFold(name, "translate"):
			first, second, count := transformArguments(args)
			isTranslate := strings.EqualFold(name, "translate")
			if isTranslate && (count < 1 || count > 2) || !isTranslate && count != 1 {
				return result, fmt.Errorf("invalid %s arguments %q", name, args)
			}
			x, err := parseLength(first, ctx)
			if err != nil {
				return result, err
			}
			y := LengthPx(0)
			if strings.EqualFold(name, "translatey") {
				y, x = x, LengthPx(0)
			} else if isTranslate && count == 2 {
				y, err = parseLength(second, ctx)
				if err != nil {
					return result, err
				}
			}
			// Compose in authored order: an earlier rotation/scale also acts on a later translation.
			result.TranslateX = addTransformLengths(result.TranslateX, x, a, stylePixelLength(y), c)
			result.TranslateY = addTransformLengths(result.TranslateY, y, d, stylePixelLength(x), b)
			result.TranslateCrossX += y.Percent * c
			result.TranslateCrossY += x.Percent * b
		case strings.EqualFold(name, "scale"), strings.EqualFold(name, "scalex"), strings.EqualFold(name, "scaley"):
			first, second, count := transformArguments(args)
			isScale := strings.EqualFold(name, "scale")
			if count < 1 || count > 2 || (!isScale && count != 1) {
				return result, fmt.Errorf("invalid scale %q", input)
			}
			x, err := parseScale(first)
			if err != nil {
				return result, err
			}
			y := x
			if count == 2 {
				y, err = parseScale(second)
				if err != nil {
					return result, err
				}
			}
			if isScale || strings.EqualFold(name, "scalex") {
				result.ScaleX *= x
				a, b = a*x, b*x
			}
			if isScale || strings.EqualFold(name, "scaley") {
				result.ScaleY *= y
				c, d = c*y, d*y
			}
		default:
			return result, fmt.Errorf("unsupported transform %q", input)
		}
		offset = close + 1
	}
	if decompose {
		sx := math.Hypot(a, b)
		determinant := a*d - b*c
		if sx == 0 || determinant == 0 {
			return result, fmt.Errorf("singular composed transform %q", input)
		}
		rotation := math.Atan2(b, a) * 180 / math.Pi
		result.Rotation = rotation + 360*math.Round((result.Rotation-rotation)/360)
		result.ScaleX, result.ScaleY = sx, determinant/sx
		result.SkewX = math.Atan((a*c+b*d)/determinant) * 180 / math.Pi
	}
	return result, nil
}

func stylePixelLength(value Value) Value { return LengthPx(value.LengthPx) }

func transformArguments(input string) (first, second string, count int) {
	start, depth := -1, 0
	for i := 0; i <= len(input); i++ {
		separator := i == len(input)
		if !separator {
			if input[i] == '(' {
				depth++
			}
			if input[i] == ')' {
				depth--
			}
			separator = depth == 0 && (isCSSWhitespace(input[i]) || input[i] == ',')
		}
		if !separator {
			if start < 0 {
				start = i
			}
			continue
		}
		if start < 0 {
			continue
		}
		value := input[start:i]
		if count == 0 {
			first = value
		} else if count == 1 {
			second = value
		}
		count++
		start = -1
	}
	return
}

func transformFunctionEnd(input string, open int) int {
	depth := 0
	for index := open; index < len(input); index++ {
		switch input[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func isCSSWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

func parseTransformRotation(input string) (float64, error) {
	trimmed := strings.TrimSpace(input)
	if strings.EqualFold(trimmed, "none") {
		return 0, nil
	}
	open := strings.IndexByte(trimmed, '(')
	if open < 0 || !strings.HasSuffix(trimmed, ")") || !strings.EqualFold(strings.TrimSpace(trimmed[:open]), "rotate") {
		return 0, fmt.Errorf("unsupported transform %q", input)
	}
	rotation, err := parseAngleDegrees(strings.TrimSpace(trimmed[open+1 : len(trimmed)-1]))
	if err == nil && (math.IsNaN(rotation) || math.IsInf(rotation, 0)) {
		return 0, fmt.Errorf("invalid transform angle %q", input)
	}
	return rotation, err
}

func parseCSSTime(input string) (float64, bool, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if len(trimmed) == 0 {
		return 0, false, nil
	}
	start := 0
	if trimmed[0] == '-' || trimmed[0] == '+' {
		start++
	}
	if start == len(trimmed) || !(trimmed[start] >= '0' && trimmed[start] <= '9' || trimmed[start] == '.') {
		return 0, false, nil
	}
	unit := ""
	switch {
	case strings.HasSuffix(trimmed, "ms"):
		unit = "ms"
	case strings.HasSuffix(trimmed, "s"):
		unit = "s"
	default:
		return 0, false, nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(trimmed, unit)), 64)
	if err != nil {
		return 0, true, fmt.Errorf("invalid transition time %q", input)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, true, fmt.Errorf("invalid transition time %q", input)
	}
	if unit == "ms" {
		value /= 1000
	}
	return value, true, nil
}

func parseScale(input string) (float64, error) {
	value := strings.TrimSpace(input)
	percent := strings.HasSuffix(value, "%")
	number, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("invalid scale %q", input)
	}
	if percent {
		number /= 100
	}
	return number, nil
}

func addTransformLengths(base, x Value, cx float64, y Value, cy float64) Value {
	px, percent := base.LengthPx, base.Percent
	if x.Kind == ValueLengthPx || x.Kind == ValueLengthCalc {
		px += x.LengthPx * cx
	}
	if x.Kind == ValueLengthPercent || x.Kind == ValueLengthCalc {
		percent += x.Percent * cx
	}
	if y.Kind == ValueLengthPx || y.Kind == ValueLengthCalc {
		px += y.LengthPx * cy
	}
	if y.Kind == ValueLengthPercent || y.Kind == ValueLengthCalc {
		percent += y.Percent * cy
	}
	if percent == 0 {
		return LengthPx(px)
	}
	if px == 0 {
		return LengthPercent(percent)
	}
	return LengthCalc(px, percent)
}

func parseTransformOrigin(input string, ctx parseContext) (Value, Value, error) {
	fields := splitCSSFields(input)
	if len(fields) < 1 || len(fields) > 2 {
		return Value{}, Value{}, fmt.Errorf("invalid transform-origin %q", input)
	}
	x, y := strings.ToLower(fields[0]), "center"
	if len(fields) == 2 {
		y = strings.ToLower(fields[1])
	}
	if x == "top" || x == "bottom" || y == "left" || y == "right" {
		x, y = y, x
	}
	resolve := func(raw string, vertical bool) (Value, error) {
		switch strings.ToLower(raw) {
		case "center":
			return LengthPercent(.5), nil
		case "left":
			if !vertical {
				return LengthPercent(0), nil
			}
		case "right":
			if !vertical {
				return LengthPercent(1), nil
			}
		case "top":
			if vertical {
				return LengthPercent(0), nil
			}
		case "bottom":
			if vertical {
				return LengthPercent(1), nil
			}
		default:
			return parseLength(raw, ctx)
		}
		return Value{}, fmt.Errorf("invalid transform-origin %q", input)
	}
	xv, err := resolve(x, false)
	if err != nil {
		return Value{}, Value{}, err
	}
	yv, err := resolve(y, true)
	return xv, yv, err
}
