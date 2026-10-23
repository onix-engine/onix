package style

import (
	"fmt"
	"strconv"
	"strings"
)

type calcValueKind uint8

const (
	calcNumber calcValueKind = iota
	calcLength
)

type calcValue struct {
	kind    calcValueKind
	number  float64
	pixels  float64
	percent float64
}

type calcParser struct {
	input string
	index int
	ctx   parseContext
}

func parseCalcLength(input string, ctx parseContext) (Value, error) {
	if len(input) < len("calc()") || !strings.HasSuffix(input, ")") {
		return Value{}, fmt.Errorf("invalid calc length %q", input)
	}
	parser := calcParser{input: input[len("calc(") : len(input)-1], ctx: ctx}
	value, err := parser.parseExpression()
	if err != nil {
		return Value{}, fmt.Errorf("invalid calc length %q: %w", input, err)
	}
	parser.skipSpaces()
	if parser.index != len(parser.input) {
		return Value{}, fmt.Errorf("invalid calc length %q: unexpected %q", input, parser.input[parser.index:])
	}
	if value.kind == calcNumber {
		if value.number == 0 {
			return LengthPx(0), nil
		}
		return Value{}, fmt.Errorf("calc expression must resolve to a length")
	}
	if value.percent == 0 {
		return LengthPx(value.pixels), nil
	}
	if value.pixels == 0 {
		return LengthPercent(value.percent), nil
	}
	return LengthCalc(value.pixels, value.percent), nil
}

func (p *calcParser) parseExpression() (calcValue, error) {
	value, err := p.parseTerm()
	if err != nil {
		return calcValue{}, err
	}
	for {
		p.skipSpaces()
		if p.consume('+') {
			right, err := p.parseTerm()
			if err != nil {
				return calcValue{}, err
			}
			value, err = addCalcValues(value, right, false)
			if err != nil {
				return calcValue{}, err
			}
			continue
		}
		if p.consume('-') {
			right, err := p.parseTerm()
			if err != nil {
				return calcValue{}, err
			}
			value, err = addCalcValues(value, right, true)
			if err != nil {
				return calcValue{}, err
			}
			continue
		}
		return value, nil
	}
}

func (p *calcParser) parseTerm() (calcValue, error) {
	value, err := p.parseUnary()
	if err != nil {
		return calcValue{}, err
	}
	for {
		p.skipSpaces()
		if p.consume('*') {
			right, err := p.parseUnary()
			if err != nil {
				return calcValue{}, err
			}
			value, err = multiplyCalcValues(value, right)
			if err != nil {
				return calcValue{}, err
			}
			continue
		}
		if p.consume('/') {
			right, err := p.parseUnary()
			if err != nil {
				return calcValue{}, err
			}
			value, err = divideCalcValues(value, right)
			if err != nil {
				return calcValue{}, err
			}
			continue
		}
		return value, nil
	}
}

func (p *calcParser) parseUnary() (calcValue, error) {
	p.skipSpaces()
	if p.consume('+') {
		return p.parseUnary()
	}
	if p.consume('-') {
		value, err := p.parseUnary()
		if err != nil {
			return calcValue{}, err
		}
		return negateCalcValue(value), nil
	}
	return p.parsePrimary()
}

func (p *calcParser) parsePrimary() (calcValue, error) {
	p.skipSpaces()
	if p.consume('(') {
		value, err := p.parseExpression()
		if err != nil {
			return calcValue{}, err
		}
		p.skipSpaces()
		if !p.consume(')') {
			return calcValue{}, fmt.Errorf("missing closing parenthesis")
		}
		return value, nil
	}
	if strings.HasPrefix(p.input[p.index:], "calc(") {
		p.index += len("calc(")
		value, err := p.parseExpression()
		if err != nil {
			return calcValue{}, err
		}
		p.skipSpaces()
		if !p.consume(')') {
			return calcValue{}, fmt.Errorf("missing closing calc parenthesis")
		}
		return value, nil
	}
	return p.parseDimension()
}

func (p *calcParser) parseDimension() (calcValue, error) {
	p.skipSpaces()
	start := p.index
	if p.index >= len(p.input) {
		return calcValue{}, fmt.Errorf("expected a value")
	}
	if p.input[p.index] == '.' {
		p.index++
	}
	digitsStart := p.index
	for p.index < len(p.input) && p.input[p.index] >= '0' && p.input[p.index] <= '9' {
		p.index++
	}
	if p.index == digitsStart && start == p.index {
		return calcValue{}, fmt.Errorf("expected a number")
	}
	if p.index < len(p.input) && p.input[p.index] == '.' {
		p.index++
		for p.index < len(p.input) && p.input[p.index] >= '0' && p.input[p.index] <= '9' {
			p.index++
		}
	}
	if p.index+1 < len(p.input) && (p.input[p.index] == 'e' || p.input[p.index] == 'E') && (p.input[p.index+1] >= '0' && p.input[p.index+1] <= '9' || p.input[p.index+1] == '+' || p.input[p.index+1] == '-') {
		p.index++
		if p.index < len(p.input) && (p.input[p.index] == '+' || p.input[p.index] == '-') {
			p.index++
		}
		exponentStart := p.index
		for p.index < len(p.input) && p.input[p.index] >= '0' && p.input[p.index] <= '9' {
			p.index++
		}
		if p.index == exponentStart {
			return calcValue{}, fmt.Errorf("invalid exponent")
		}
	}
	number, err := strconv.ParseFloat(p.input[start:p.index], 64)
	if err != nil {
		return calcValue{}, fmt.Errorf("parse number %q: %w", p.input[start:p.index], err)
	}
	unitStart := p.index
	if p.index < len(p.input) && p.input[p.index] == '%' {
		p.index++
		return calcValue{kind: calcLength, percent: number / 100}, nil
	}
	for p.index < len(p.input) && ((p.input[p.index] >= 'a' && p.input[p.index] <= 'z') || (p.input[p.index] >= 'A' && p.input[p.index] <= 'Z')) {
		p.index++
	}
	unit := strings.ToLower(p.input[unitStart:p.index])
	if unit == "" {
		return calcValue{kind: calcNumber, number: number}, nil
	}
	switch unit {
	case "px":
		return calcValue{kind: calcLength, pixels: number}, nil
	case "em":
		fontSize := p.ctx.FontSize
		if fontSize <= 0 {
			fontSize = 16
		}
		return calcValue{kind: calcLength, pixels: number * fontSize}, nil
	case "rem":
		rootFontSize := p.ctx.RootFontSize
		if rootFontSize <= 0 {
			rootFontSize = 16
		}
		return calcValue{kind: calcLength, pixels: number * rootFontSize}, nil
	case "vw":
		return calcValue{kind: calcLength, pixels: p.ctx.Viewport.Width * number / 100}, nil
	case "vh":
		return calcValue{kind: calcLength, pixels: p.ctx.Viewport.Height * number / 100}, nil
	default:
		return calcValue{}, fmt.Errorf("unsupported unit %q", unit)
	}
}

func (p *calcParser) skipSpaces() {
	for p.index < len(p.input) {
		switch p.input[p.index] {
		case ' ', '\t', '\n', '\r':
			p.index++
		default:
			return
		}
	}
}

func (p *calcParser) consume(value byte) bool {
	if p.index >= len(p.input) || p.input[p.index] != value {
		return false
	}
	p.index++
	return true
}

func addCalcValues(left, right calcValue, subtract bool) (calcValue, error) {
	if left.kind == calcNumber && left.number == 0 {
		if subtract {
			return negateCalcValue(right), nil
		}
		return right, nil
	}
	if right.kind == calcNumber && right.number == 0 {
		return left, nil
	}
	if left.kind != right.kind {
		return calcValue{}, fmt.Errorf("cannot add a number and a length")
	}
	if subtract {
		right = negateCalcValue(right)
	}
	if left.kind == calcNumber {
		left.number += right.number
		return left, nil
	}
	left.pixels += right.pixels
	left.percent += right.percent
	return left, nil
}

func multiplyCalcValues(left, right calcValue) (calcValue, error) {
	if left.kind == calcNumber {
		return scaleCalcValue(right, left.number), nil
	}
	if right.kind == calcNumber {
		return scaleCalcValue(left, right.number), nil
	}
	return calcValue{}, fmt.Errorf("cannot multiply two lengths")
}

func divideCalcValues(left, right calcValue) (calcValue, error) {
	if right.kind != calcNumber {
		return calcValue{}, fmt.Errorf("length division requires a number")
	}
	if right.number == 0 {
		return calcValue{}, fmt.Errorf("division by zero")
	}
	return scaleCalcValue(left, 1/right.number), nil
}

func scaleCalcValue(value calcValue, factor float64) calcValue {
	if value.kind == calcNumber {
		value.number *= factor
		return value
	}
	value.pixels *= factor
	value.percent *= factor
	return value
}

func negateCalcValue(value calcValue) calcValue {
	return scaleCalcValue(value, -1)
}
