package style

import "hash/maphash"

type motionInputs struct {
	transition [4]string
	animation  [8]string
}

// motionStyle is immutable after interning. Static styles have no extension;
// unrelated layout/paint styles can share the same resolved motion values.
type motionStyle struct {
	motionInputs
	animations  []CSSAnimation
	transitions [AnimatedPropertyCount]TransformTransition
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
	motion.transitions = resolveTransitions(motion.transition)
	motion.animations = resolveAnimations(motion.animation)
	motion.shared = true
}

// Prepared motion declarations contain only the fields their property sets.
// Merge raw longhands before resolving lists, preserving cascade and inherit.
func applyPreparedMotion(computed *ComputedStyle, property propertyID, inputs *motionInputs) {
	motion := computed.mutableMotion()
	switch {
	case property == propertyTransition:
		motion.transition = inputs.transition
	case property == propertyAnimation:
		motion.animation = inputs.animation
	case property.isTransition():
		i := property - propertyTransitionProperty
		motion.transition[i] = inputs.transition[i]
	case property.isAnimation():
		i := property - propertyAnimationName
		motion.animation[i] = inputs.animation[i]
	}
}
