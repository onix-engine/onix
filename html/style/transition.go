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

// Resolve each list entry once, then apply it to all matching properties.
// Later entries replace earlier matches, including shorthand matches.
func resolveTransitions(values [4]string) (result [AnimatedPropertyCount]TransformTransition) {
	if values == ([4]string{}) {
		return result
	}
	var lists [4][]string
	for i, value := range values {
		if value == "" {
			value = transitionInitial[i]
		}
		lists[i] = splitCSSList(value)
	}
	for i, name := range lists[0] {
		name = strings.ToLower(strings.TrimSpace(name))
		properties := AnimatedProperty(name)
		if name == "all" {
			properties = AnimatedTransform | AnimatedOpacity | AnimatedColors
		}
		if properties == 0 {
			continue
		}
		transition := TransformTransition{Enabled: true}
		transition.Duration, _, _ = parseCSSTime(lists[1][i%len(lists[1])])
		transition.Timing, _ = ParseTimingFunction(lists[2][i%len(lists[2])])
		transition.Delay, _, _ = parseCSSTime(lists[3][i%len(lists[3])])
		for index, property := range AnimatedPropertyList {
			if properties&property != 0 {
				result[index] = transition
			}
		}
	}
	return result
}
