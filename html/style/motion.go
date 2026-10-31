package style

import (
	"hash/maphash"
	"slices"
)

type motionInputs struct {
	transition [4]string
	animation  [8]string
}

// motionStyle is immutable after interning. Static styles have no extension;
// unrelated layout/paint styles can share the same resolved motion values.
type motionStyle struct {
	motionInputs
	transitions [AnimatedPropertyCount]TransformTransition
	animations  []CSSAnimation
	shared      bool
}

func (computed *ComputedStyle) mutableMotion() *motionStyle {
	if computed.motion == nil {
		computed.motion = new(motionStyle)
	} else if computed.motion.shared {
		copy := *computed.motion
		copy.shared = false
		computed.motion = &copy
	}
	return computed.motion
}

func (computed *ComputedStyle) transitionValues() [4]string {
	if computed.motion == nil {
		return [4]string{}
	}
	return computed.motion.transition
}

func (computed *ComputedStyle) animationValues() [8]string {
	if computed.motion == nil {
		return [8]string{}
	}
	return computed.motion.animation
}

// Animations returns immutable animation definitions shared by computed styles.
func (computed *ComputedStyle) Animations() []CSSAnimation {
	if computed.motion == nil {
		return nil
	}
	return computed.motion.animations
}

func (computed *ComputedStyle) HasMotion() bool {
	if computed.motion == nil {
		return false
	}
	for _, animation := range computed.motion.animations {
		if animation.Name != "" {
			return true
		}
	}
	for _, transition := range computed.motion.transitions {
		if transition.Enabled {
			return true
		}
	}
	return false
}

func motionEqual(a, b *motionStyle) bool {
	return a == b || a != nil && b != nil && a.motionInputs == b.motionInputs
}

func motionHash(seed maphash.Seed, motion *motionStyle) uint64 {
	return maphash.Comparable(seed, motion.motionInputs)
}

func resolveMotion(motion *motionStyle) {
	motion.transitions = [AnimatedPropertyCount]TransformTransition{}
	if motion.transition != ([4]string{}) {
		for i, property := range AnimatedPropertyList {
			name := "transform"
			if property == AnimatedOpacity {
				name = "opacity"
			} else if i >= 2 {
				name = animatedColorNames[i-2]
			}
			motion.transitions[i] = resolveTransition(motion.transition, name)
		}
	}
	motion.animations = resolveAnimations(motion.animation)
	motion.shared = true
}

// Prepared motion declarations contain only the fields their property sets.
// Merge raw longhands before resolving lists, preserving cascade and inherit.
func applyPreparedMotion(computed *ComputedStyle, parsed *preparedValue) {
	motion := computed.mutableMotion()
	switch parsed.property {
	case "transition":
		motion.transition = parsed.motion.transition
	case "animation":
		motion.animation = parsed.motion.animation
	default:
		if index := slices.Index(transitionProperties[:], parsed.property); index >= 0 {
			motion.transition[index] = parsed.motion.transition[index]
		} else if index := slices.Index(animationProperties[:], parsed.property); index >= 0 {
			motion.animation[index] = parsed.motion.animation[index]
		}
	}
}
