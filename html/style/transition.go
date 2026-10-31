package style

import (
	"fmt"
	"slices"
	"strings"
)

var transitionProperties = [...]string{"transition-property", "transition-duration", "transition-timing-function", "transition-delay"}
var transitionInitial = [...]string{"all", "0s", "ease", "0s"}

func applyTransitionProperty(computed *ComputedStyle, property, raw string) error {
	values := computed.transitionValues()
	if property == "transition" {
		var builders [4]strings.Builder
		entries := splitCSSList(raw)
		if len(entries) == 0 {
			return fmt.Errorf("empty transition")
		}
		for item, entry := range entries {
			parts := transitionInitial
			var assigned [4]bool
			for _, field := range splitCSSFields(entry) {
				index := 0
				if _, valid, err := parseCSSTime(field); valid || err != nil {
					index = 1
					if assigned[index] {
						index = 3
					}
				} else if _, valid := ParseTimingFunction(field); valid {
					index = 2
				}
				if assigned[index] {
					return fmt.Errorf("duplicate transition value %q", field)
				}
				if err := validateTransitionField(index, field); err != nil {
					return err
				}
				parts[index], assigned[index] = field, true
			}
			if parts[0] == "none" && len(entries) != 1 {
				return fmt.Errorf("none in transition list")
			}
			for i, part := range parts {
				if item > 0 {
					builders[i].WriteByte(',')
				}
				builders[i].WriteString(part)
			}
		}
		for i := range builders {
			values[i] = builders[i].String()
		}
	} else {
		index := slices.Index(transitionProperties[:], property)
		if index < 0 {
			return nil
		}
		entries := splitCSSList(raw)
		if len(entries) == 0 {
			return fmt.Errorf("empty transition value")
		}
		for _, entry := range entries {
			if index == 0 && strings.EqualFold(strings.TrimSpace(entry), "none") && len(entries) != 1 {
				return fmt.Errorf("none in transition-property list")
			}
			if err := validateTransitionField(index, entry); err != nil {
				return err
			}
		}
		values[index] = raw
	}
	computed.mutableMotion().transition = values
	return nil
}

func validateTransitionField(index int, raw string) error {
	value := strings.TrimSpace(raw)
	switch index {
	case 0:
		for i, r := range value {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_' || i > 0 && r >= '0' && r <= '9') {
				return fmt.Errorf("invalid transition property %q", raw)
			}
		}
		if value == "" {
			return fmt.Errorf("empty transition property")
		}
	case 1, 3:
		time, valid, err := parseCSSTime(value)
		if err != nil || !valid || index == 1 && time < 0 {
			return fmt.Errorf("invalid transition time %q", raw)
		}
	case 2:
		if _, valid := ParseTimingFunction(value); !valid {
			return fmt.Errorf("invalid transition timing %q", raw)
		}
	}
	return nil
}

func resolveTransition(values [4]string, property string) TransformTransition {
	var lists [4][]string
	for i, value := range values {
		if value == "" {
			value = transitionInitial[i]
		}
		lists[i] = splitCSSList(value)
	}
	result := TransformTransition{}
	for i, name := range lists[0] {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "all" && name != property && AnimatedProperty(name)&AnimatedProperty(property) == 0 {
			continue
		}
		result.Enabled = true
		result.Duration, _, _ = parseCSSTime(lists[1][i%len(lists[1])])
		result.Timing, _ = ParseTimingFunction(lists[2][i%len(lists[2])])
		result.Easing = result.Timing.Kind
		result.Delay, _, _ = parseCSSTime(lists[3][i%len(lists[3])])
	}
	return result
}

func parseTransformTransition(input string) (TransformTransition, error) {
	value := ComputedStyle{}
	err := applyTransitionProperty(&value, "transition", input)
	return resolveTransition(value.transitionValues(), "transform"), err
}
