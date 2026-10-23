package style

import (
	"math"
	"strconv"
	"strings"
)

// TimingFunction is a compiled CSS easing function. Sampling allocates nothing.
type TimingFunction struct {
	Kind           TransformEasing
	X1, Y1, X2, Y2 float64
	Steps          int
	Jump           string
}

const (
	TransformCubicBezier TransformEasing = TransformEaseInOut + 1 + iota
	TransformSteps
)

func ParseTimingFunction(input string) (TimingFunction, bool) {
	value := strings.ToLower(strings.TrimSpace(input))
	result := TimingFunction{}
	switch value {
	case "ease":
		result.Kind = TransformEase
	case "linear":
		result.Kind = TransformLinear
	case "ease-in":
		result.Kind = TransformEaseIn
	case "ease-out":
		result.Kind = TransformEaseOut
	case "ease-in-out":
		result.Kind = TransformEaseInOut
	case "step-start":
		result.Kind, result.Steps, result.Jump = TransformSteps, 1, "jump-start"
	case "step-end":
		result.Kind, result.Steps, result.Jump = TransformSteps, 1, "jump-end"
	default:
		if strings.HasPrefix(value, "cubic-bezier(") && strings.HasSuffix(value, ")") {
			args := splitCSSList(value[len("cubic-bezier(") : len(value)-1])
			if len(args) != 4 {
				return result, false
			}
			coordinates := [4]float64{}
			for i, arg := range args {
				number, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
				if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
					return result, false
				}
				coordinates[i] = number
			}
			if coordinates[0] < 0 || coordinates[0] > 1 || coordinates[2] < 0 || coordinates[2] > 1 {
				return result, false
			}
			result.Kind = TransformCubicBezier
			result.X1, result.Y1, result.X2, result.Y2 = coordinates[0], coordinates[1], coordinates[2], coordinates[3]
		} else if strings.HasPrefix(value, "steps(") && strings.HasSuffix(value, ")") {
			args := splitCSSList(value[len("steps(") : len(value)-1])
			if len(args) < 1 || len(args) > 2 {
				return result, false
			}
			count, err := strconv.Atoi(strings.TrimSpace(args[0]))
			if err != nil || count <= 0 {
				return result, false
			}
			jump := "jump-end"
			if len(args) == 2 {
				jump = strings.TrimSpace(args[1])
			}
			switch jump {
			case "start":
				jump = "jump-start"
			case "end":
				jump = "jump-end"
			case "jump-start", "jump-end", "jump-both":
			case "jump-none":
				if count < 2 {
					return result, false
				}
			default:
				return result, false
			}
			result.Kind, result.Steps, result.Jump = TransformSteps, count, jump
		} else {
			return result, false
		}
	}
	return result, true
}

func (f TimingFunction) Sample(progress float64, before bool) float64 {
	progress = min(max(progress, 0), 1)
	if f.Kind == TransformLinear {
		return progress
	}
	if f.Kind == TransformSteps {
		step, jumps := math.Floor(progress*float64(f.Steps)), f.Steps
		switch f.Jump {
		case "jump-start":
			step++
		case "jump-both":
			step++
			jumps++
		case "jump-none":
			jumps--
		}
		if before && progress*float64(f.Steps) == math.Floor(progress*float64(f.Steps)) {
			step--
		}
		return min(max(step/float64(jumps), 0), 1)
	}
	if progress == 0 || progress == 1 {
		return progress
	}
	x1, y1, x2, y2 := f.X1, f.Y1, f.X2, f.Y2
	switch f.Kind {
	case TransformEase:
		x1, y1, x2, y2 = .25, .1, .25, 1
	case TransformEaseIn:
		x1, y1, x2, y2 = .42, 0, 1, 1
	case TransformEaseOut:
		x1, y1, x2, y2 = 0, 0, .58, 1
	case TransformEaseInOut:
		x1, y1, x2, y2 = .42, 0, .58, 1
	}
	lo, hi := float64(0), float64(1)
	for range 30 {
		mid := (lo + hi) / 2
		if cubicCoordinate(mid, x1, x2) < progress {
			lo = mid
		} else {
			hi = mid
		}
	}
	return cubicCoordinate((lo+hi)/2, y1, y2)
}

func cubicCoordinate(t, a, b float64) float64 { return 3*(1-t)*(1-t)*t*a + 3*(1-t)*t*t*b + t*t*t }
