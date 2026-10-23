package style

func IsLength(value Value) bool {
	return value.Kind == ValueLengthPx || value.Kind == ValueLengthPercent || value.Kind == ValueLengthCalc
}

func ResolveLength(value Value, base float64) float64 {
	switch value.Kind {
	case ValueLengthPx:
		return value.LengthPx
	case ValueLengthPercent:
		return base * value.Percent
	case ValueLengthCalc:
		return value.LengthPx + base*value.Percent
	default:
		return 0
	}
}
