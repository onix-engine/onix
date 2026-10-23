package css

func CalculateSpecificity(selector Selector) Specificity {
	var specificity Specificity
	for _, part := range selector.Parts {
		if part.ID != "" {
			specificity.ID++
		}
		specificity.Class += len(part.Classes)
		specificity.Class += len(part.Attributes)
		for _, pseudoClass := range part.PseudoClasses {
			specificity = addSpecificity(specificity, pseudoClassSpecificity(pseudoClass))
		}
		if part.Tag != "" && part.Tag != "*" {
			specificity.Tag++
		}
	}
	return specificity
}

func pseudoClassSpecificity(pseudoClass PseudoClass) Specificity {
	switch pseudoClass.Name {
	case "where":
		return Specificity{}
	case "not", "is":
		return maxSelectorSpecificity(pseudoClass.Selectors)
	default:
		return Specificity{Class: 1}
	}
}

func maxSelectorSpecificity(selectors []Selector) Specificity {
	var max Specificity
	for _, selector := range selectors {
		current := CalculateSpecificity(selector)
		if compareSpecificity(current, max) > 0 {
			max = current
		}
	}
	return max
}

func compareSpecificity(left, right Specificity) int {
	switch {
	case left.ID != right.ID:
		return left.ID - right.ID
	case left.Class != right.Class:
		return left.Class - right.Class
	default:
		return left.Tag - right.Tag
	}
}

func addSpecificity(left, right Specificity) Specificity {
	return Specificity{ID: left.ID + right.ID, Class: left.Class + right.Class, Tag: left.Tag + right.Tag}
}
