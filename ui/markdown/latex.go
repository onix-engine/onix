package markdown

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/util"
)

var latexSymbols = map[string]string{
	"alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ", "epsilon": "ε", "varepsilon": "ϵ",
	"zeta": "ζ", "eta": "η", "theta": "θ", "vartheta": "ϑ", "iota": "ι", "kappa": "κ", "lambda": "λ",
	"mu": "μ", "nu": "ν", "xi": "ξ", "pi": "π", "varpi": "ϖ", "rho": "ρ", "sigma": "σ", "tau": "τ",
	"upsilon": "υ", "phi": "φ", "varphi": "ϕ", "chi": "χ", "psi": "ψ", "omega": "ω",
	"Gamma": "Γ", "Delta": "Δ", "Theta": "Θ", "Lambda": "Λ", "Xi": "Ξ", "Pi": "Π", "Sigma": "Σ", "Upsilon": "Υ", "Phi": "Φ", "Psi": "Ψ", "Omega": "Ω",
	"times": "×", "cdot": "·", "div": "÷", "pm": "±", "mp": "∓", "le": "≤", "leq": "≤", "ge": "≥", "geq": "≥",
	"ne": "≠", "neq": "≠", "approx": "≈", "equiv": "≡", "infty": "∞", "sum": "∑", "prod": "∏", "int": "∫",
	"oint": "∮", "iint": "∬", "iiint": "∭", "partial": "∂", "nabla": "∇", "in": "∈", "notin": "∉",
	"subset": "⊂", "subseteq": "⊆", "supset": "⊃", "supseteq": "⊇", "cup": "∪", "cap": "∩",
	"forall": "∀", "exists": "∃", "neg": "¬", "land": "∧", "lor": "∨", "wedge": "∧", "vee": "∨",
	"to": "→", "rightarrow": "→", "leftarrow": "←", "leftrightarrow": "↔", "Rightarrow": "⇒", "Leftarrow": "⇐", "Leftrightarrow": "⇔",
	"Longrightarrow": "⟹", "Longleftarrow": "⟸", "Longleftrightarrow": "⟺", "mapsto": "↦",
	"uparrow": "↑", "downarrow": "↓", "updownarrow": "↕", "Uparrow": "⇑", "Downarrow": "⇓", "Updownarrow": "⇕",
	"ldots": "…", "cdots": "⋯", "vdots": "⋮", "ddots": "⋱", "angle": "∠", "degree": "°", "prime": "′", "hbar": "ℏ", "ell": "ℓ",
	"langle": "⟨", "rangle": "⟩", "lceil": "⌈", "rceil": "⌉", "lfloor": "⌊", "rfloor": "⌋",
	"vert": "|", "lvert": "|", "rvert": "|", "Vert": "‖", "lVert": "‖", "rVert": "‖",
	"sim": "∼", "simeq": "≃", "cong": "≅", "propto": "∝", "mid": "∣", "parallel": "∥", "perp": "⊥",
	"oplus": "⊕", "otimes": "⊗", "ominus": "⊖", "oslash": "⊘", "odot": "⊙", "circ": "∘", "bullet": "∙", "emptyset": "∅",
	"sin": "sin", "cos": "cos", "tan": "tan", "cot": "cot", "sec": "sec", "csc": "csc",
	"sinh": "sinh", "cosh": "cosh", "tanh": "tanh", "arcsin": "arcsin", "arccos": "arccos", "arctan": "arctan",
	"log": "log", "ln": "ln", "exp": "exp", "lim": "lim", "min": "min", "max": "max", "det": "det",
	"lbrace": "{", "rbrace": "}", "lbrack": "[", "rbrack": "]", "colon": ":", "quad": "  ", "qquad": "    ",
}

type mathNodeKind uint8

const (
	mathEmpty mathNodeKind = iota
	mathSequence
	mathText
	mathFraction
	mathRadical
	mathScripts
	mathMatrix
	mathStyle
)

type mathNode struct {
	kind       mathNodeKind
	value      string
	name       string
	left       string
	right      string
	noBar      bool
	children   []mathNode
	rows       [][]mathNode
	alignments []mathAlignment
}

type mathAlignment uint8

const (
	mathAlignCenter mathAlignment = iota
	mathAlignLeft
	mathAlignRight
)

type mathParser struct {
	value string
	index int
}

func parseMarkdownMath(value []byte) mathNode {
	parser := mathParser{value: strings.TrimSpace(string(value))}
	return parser.sequence(0)
}

func (parser *mathParser) sequence(stop byte) mathNode {
	children := make([]mathNode, 0, 8)
	space := false
	for parser.index < len(parser.value) {
		character := parser.value[parser.index]
		if stop != 0 && character == stop {
			parser.index++
			break
		}
		if character == '%' {
			for parser.index < len(parser.value) && parser.value[parser.index] != '\n' {
				parser.index++
			}
			continue
		}
		if character == '^' || character == '_' {
			if len(children) == 0 {
				parser.index++
				continue
			}
			children[len(children)-1] = parser.scripts(children[len(children)-1])
			continue
		}
		r, size := utf8.DecodeRuneInString(parser.value[parser.index:])
		if unicode.IsSpace(r) {
			parser.index += size
			space = len(children) > 0
			continue
		}
		if space {
			children = appendMathText(children, " ")
			space = false
		}

		var node mathNode
		switch character {
		case '{':
			parser.index++
			node = parser.sequence('}')
		case '}':
			parser.index++
			return mathNode{kind: mathSequence, children: children}
		case '\\':
			node = parser.command()
			if node.kind == mathText && node.value == "\x00over" {
				return mathNode{kind: mathFraction, children: []mathNode{{kind: mathSequence, children: children}, parser.sequence(stop)}}
			}
		default:
			start := parser.index
			for parser.index < len(parser.value) {
				current, currentSize := utf8.DecodeRuneInString(parser.value[parser.index:])
				if unicode.IsSpace(current) || strings.ContainsRune("{}\\^_", current) {
					break
				}
				parser.index += currentSize
			}
			node = mathNode{kind: mathText, value: parser.value[start:parser.index]}
		}
		if node.kind == mathEmpty {
			continue
		}
		beforeScripts := parser.index
		parser.skipSpaces()
		if parser.index < len(parser.value) && (parser.value[parser.index] == '^' || parser.value[parser.index] == '_') {
			node = parser.scripts(node)
		} else {
			parser.index = beforeScripts
		}
		if node.kind == mathText {
			children = appendMathText(children, node.value)
		} else {
			children = append(children, node)
		}
	}
	return mathNode{kind: mathSequence, children: children}
}

func (parser *mathParser) scripts(base mathNode) mathNode {
	children := []mathNode{base, {}, {}}
	for parser.index < len(parser.value) && (parser.value[parser.index] == '^' || parser.value[parser.index] == '_') {
		marker := parser.value[parser.index]
		parser.index++
		script := parser.argument()
		if marker == '^' {
			children[1] = script
		} else {
			children[2] = script
		}
		parser.skipSpaces()
	}
	return mathNode{kind: mathScripts, children: children}
}

func (parser *mathParser) command() mathNode {
	parser.index++
	if parser.index >= len(parser.value) {
		return mathNode{}
	}
	start := parser.index
	for parser.index < len(parser.value) {
		r, size := utf8.DecodeRuneInString(parser.value[parser.index:])
		if !unicode.IsLetter(r) {
			break
		}
		parser.index += size
	}
	if start == parser.index {
		_, size := utf8.DecodeRuneInString(parser.value[parser.index:])
		value := parser.value[parser.index : parser.index+size]
		parser.index += size
		if strings.Contains(",;:!\\", value) {
			value = " "
		}
		return mathNode{kind: mathText, value: value}
	}
	command := parser.value[start:parser.index]
	switch command {
	case "frac", "dfrac", "tfrac", "cfrac":
		return mathNode{kind: mathFraction, children: []mathNode{parser.argument(), parser.argument()}}
	case "binom", "tbinom", "dbinom":
		return mathNode{kind: mathFraction, children: []mathNode{parser.argument(), parser.argument()}, left: "(", right: ")", noBar: true}
	case "over":
		return mathNode{kind: mathText, value: "\x00over"}
	case "sqrt":
		index := parser.optionalArgument()
		return mathNode{kind: mathRadical, children: []mathNode{parser.argument(), index}}
	case "overset", "stackrel":
		top := parser.argument()
		return mathNode{kind: mathScripts, children: []mathNode{parser.argument(), top, {}}}
	case "underset":
		bottom := parser.argument()
		return mathNode{kind: mathScripts, children: []mathNode{parser.argument(), {}, bottom}}
	case "text", "textrm", "textnormal", "textsf", "texttt", "textsc", "operatorname", "mathop":
		if parser.index < len(parser.value) && parser.value[parser.index] == '*' {
			parser.index++
		}
		return mathNode{kind: mathText, value: parser.rawArgument()}
	case "mathbf", "boldsymbol", "bm", "textbf":
		return mathNode{kind: mathStyle, name: "bold", children: []mathNode{parser.argument()}}
	case "mathit", "textit", "emph":
		return mathNode{kind: mathStyle, name: "italic", children: []mathNode{parser.argument()}}
	case "mathrm":
		return mathNode{kind: mathStyle, name: "roman", children: []mathNode{parser.argument()}}
	case "mathtt":
		return mathNode{kind: mathStyle, name: "mono", children: []mathNode{parser.argument()}}
	case "mathbb", "Bbb", "mathcal", "mathscr", "mathfrak", "mathsf":
		return mathNode{kind: mathStyle, name: command, children: []mathNode{parser.argument()}}
	case "hat", "widehat", "bar", "overline", "vec", "dot", "ddot", "underline", "tilde", "widetilde":
		return mathNode{kind: mathStyle, name: command, children: []mathNode{parser.argument()}}
	case "pmod":
		return mathNode{kind: mathSequence, children: []mathNode{{kind: mathText, value: "(mod "}, parser.argument(), {kind: mathText, value: ")"}}}
	case "bmod":
		return mathNode{kind: mathText, value: " mod "}
	case "color":
		_ = parser.optionalArgument()
		_ = parser.argument()
		return mathNode{}
	case "textcolor":
		_ = parser.argument()
		return parser.argument()
	case "displaystyle", "textstyle", "scriptstyle", "scriptscriptstyle", "limits", "nolimits":
		return mathNode{}
	case "big", "Big", "bigg", "Bigg", "bigl", "bigr", "Bigl", "Bigr", "biggl", "biggr", "Biggl", "Biggr":
		return parser.argument()
	case "left", "right":
		return parser.delimiter()
	case "begin":
		return parser.environment()
	}
	if symbol, ok := latexSymbols[command]; ok {
		return mathNode{kind: mathText, value: symbol}
	}
	return mathNode{kind: mathText, value: command}
}

func (parser *mathParser) argument() mathNode {
	parser.skipSpaces()
	if parser.index >= len(parser.value) {
		return mathNode{}
	}
	if parser.value[parser.index] == '{' {
		parser.index++
		return parser.sequence('}')
	}
	if parser.value[parser.index] == '\\' {
		return parser.command()
	}
	_, size := utf8.DecodeRuneInString(parser.value[parser.index:])
	value := parser.value[parser.index : parser.index+size]
	parser.index += size
	return mathNode{kind: mathText, value: value}
}

func (parser *mathParser) optionalArgument() mathNode {
	parser.skipSpaces()
	if parser.index >= len(parser.value) || parser.value[parser.index] != '[' {
		return mathNode{}
	}
	parser.index++
	start, braces := parser.index, 0
	for parser.index < len(parser.value) {
		switch parser.value[parser.index] {
		case '{':
			braces++
		case '}':
			braces = max(braces-1, 0)
		case ']':
			if braces == 0 {
				value := parser.value[start:parser.index]
				parser.index++
				return parseMarkdownMath([]byte(value))
			}
		}
		parser.index++
	}
	return parseMarkdownMath([]byte(parser.value[start:]))
}

func (parser *mathParser) rawArgument() string {
	parser.skipSpaces()
	if parser.index >= len(parser.value) || parser.value[parser.index] != '{' {
		node := parser.argument()
		if node.kind == mathText {
			return node.value
		}
		return ""
	}
	parser.index++
	start, depth := parser.index, 1
	for parser.index < len(parser.value) {
		if parser.value[parser.index] == '\\' {
			parser.index++
			if parser.index < len(parser.value) {
				_, size := utf8.DecodeRuneInString(parser.value[parser.index:])
				parser.index += size
			}
			continue
		}
		switch parser.value[parser.index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				value := parser.value[start:parser.index]
				parser.index++
				return value
			}
		}
		parser.index++
	}
	return parser.value[start:]
}

func (parser *mathParser) delimiter() mathNode {
	parser.skipSpaces()
	if parser.index >= len(parser.value) {
		return mathNode{}
	}
	if parser.value[parser.index] == '\\' {
		return parser.command()
	}
	_, size := utf8.DecodeRuneInString(parser.value[parser.index:])
	value := parser.value[parser.index : parser.index+size]
	parser.index += size
	if value == "." {
		return mathNode{}
	}
	return mathNode{kind: mathText, value: value}
}

func (parser *mathParser) environment() mathNode {
	name := strings.TrimSpace(parser.rawArgument())
	if name == "alignedat" || name == "alignedat*" || name == "alignat" || name == "alignat*" {
		_ = parser.rawArgument()
	}
	bodyStart := parser.index
	bodyEnd, environmentEnd, ok := findMathEnvironmentEnd(parser.value, name, bodyStart)
	if !ok || !knownMathEnvironment(name) {
		return mathNode{kind: mathText, value: name}
	}
	body := parser.value[bodyStart:bodyEnd]
	parser.index = environmentEnd
	alignments := []mathAlignment(nil)
	if name == "array" {
		body, alignments = parseMathArrayAlignment(body)
	}
	rows := make([][]mathNode, 0, 4)
	for _, rawRow := range splitMathTopLevel(body, `\\`) {
		if strings.TrimSpace(rawRow) == "" {
			continue
		}
		rawCells := splitMathTopLevel(rawRow, "&")
		row := make([]mathNode, len(rawCells))
		for index, cell := range rawCells {
			row[index] = parseMarkdownMath([]byte(cell))
		}
		rows = append(rows, row)
	}
	return mathNode{kind: mathMatrix, name: name, rows: rows, alignments: alignments}
}

func (parser *mathParser) skipSpaces() {
	for parser.index < len(parser.value) {
		r, size := utf8.DecodeRuneInString(parser.value[parser.index:])
		if !unicode.IsSpace(r) {
			return
		}
		parser.index += size
	}
}

func appendMathText(nodes []mathNode, value string) []mathNode {
	if value == "" {
		return nodes
	}
	if len(nodes) > 0 && nodes[len(nodes)-1].kind == mathText {
		nodes[len(nodes)-1].value += value
		return nodes
	}
	return append(nodes, mathNode{kind: mathText, value: value})
}

func mathEnvironmentToken(value string, start int, prefix string) (string, int, bool) {
	if start < 0 || !strings.HasPrefix(value[start:], prefix) {
		return "", start, false
	}
	nameStart := start + len(prefix)
	nameEnd := strings.IndexByte(value[nameStart:], '}')
	if nameEnd < 0 {
		return "", len(value), false
	}
	nameEnd += nameStart
	name := strings.TrimSpace(value[nameStart:nameEnd])
	return name, nameEnd + 1, name != ""
}

func findMathEnvironmentEnd(value, root string, bodyStart int) (int, int, bool) {
	stack := []string{root}
	for cursor := bodyStart; cursor < len(value); {
		if name, next, ok := mathEnvironmentToken(value, cursor, `\begin{`); ok {
			stack = append(stack, name)
			cursor = next
			continue
		}
		if name, next, ok := mathEnvironmentToken(value, cursor, `\end{`); ok {
			if name == stack[len(stack)-1] {
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					return cursor, next, true
				}
			}
			cursor = next
			continue
		}
		if value[cursor] == '%' {
			for cursor < len(value) && value[cursor] != '\n' {
				cursor++
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(value[cursor:])
		cursor += size
	}
	return 0, 0, false
}

func knownMathEnvironment(name string) bool {
	switch name {
	case "matrix", "pmatrix", "bmatrix", "Bmatrix", "vmatrix", "Vmatrix", "smallmatrix", "cases", "aligned", "align", "align*", "array",
		"alignedat", "alignedat*", "alignat", "alignat*", "gathered", "gather", "gather*", "split", "equation", "equation*", "multline", "multline*":
		return true
	default:
		return false
	}
}

func splitMathTopLevel(value, delimiter string) []string {
	result := make([]string, 0, 4)
	start, braces, environments := 0, 0, 0
	for index := 0; index < len(value); {
		if _, next, ok := mathEnvironmentToken(value, index, `\begin{`); ok {
			environments++
			index = next
			continue
		}
		if _, next, ok := mathEnvironmentToken(value, index, `\end{`); ok {
			environments = max(environments-1, 0)
			index = next
			continue
		}
		if braces == 0 && environments == 0 && strings.HasPrefix(value[index:], delimiter) {
			result = append(result, strings.TrimSpace(value[start:index]))
			index += len(delimiter)
			start = index
			continue
		}
		if value[index] == '%' {
			for index < len(value) && value[index] != '\n' {
				index++
			}
			continue
		}
		if value[index] == '\\' && index+1 < len(value) {
			index += 2
			continue
		}
		switch value[index] {
		case '{':
			braces++
		case '}':
			braces = max(braces-1, 0)
		}
		_, size := utf8.DecodeRuneInString(value[index:])
		index += size
	}
	return append(result, strings.TrimSpace(value[start:]))
}

func parseMathArrayAlignment(body string) (string, []mathAlignment) {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "{") {
		return body, nil
	}
	end := strings.IndexByte(body, '}')
	if end < 0 {
		return body, nil
	}
	alignments := make([]mathAlignment, 0, end-1)
	for _, character := range body[1:end] {
		switch character {
		case 'l':
			alignments = append(alignments, mathAlignLeft)
		case 'r':
			alignments = append(alignments, mathAlignRight)
		case 'c':
			alignments = append(alignments, mathAlignCenter)
		}
	}
	return strings.TrimSpace(body[end+1:]), alignments
}

func writeMarkdownMathHTML(writer util.BufWriter, source []byte) {
	writeMathNodeHTML(writer, parseMarkdownMath(source))
}

func writeMathNodeHTML(writer util.BufWriter, node mathNode) {
	switch node.kind {
	case mathText:
		_, _ = writer.Write(util.EscapeHTML([]byte(node.value)))
	case mathSequence:
		for _, child := range node.children {
			writeMathNodeHTML(writer, child)
		}
	case mathFraction:
		writeMathTextHTML(writer, node.left)
		class := `math-frac`
		if node.noBar {
			class += ` math-frac-no-bar`
		}
		_, _ = writer.WriteString(`<span class="` + class + `"><span class="math-num">`)
		writeMathChildHTML(writer, node, 0)
		_, _ = writer.WriteString(`</span><span class="math-den">`)
		writeMathChildHTML(writer, node, 1)
		_, _ = writer.WriteString(`</span></span>`)
		writeMathTextHTML(writer, node.right)
	case mathRadical:
		_, _ = writer.WriteString(`<span class="math-radical">`)
		if len(node.children) > 1 && node.children[1].kind != mathEmpty {
			_, _ = writer.WriteString(`<span class="math-root-index">`)
			writeMathNodeHTML(writer, node.children[1])
			_, _ = writer.WriteString(`</span>`)
		}
		_, _ = writer.WriteString(`<span class="math-root-symbol">√</span><span class="math-radicand"><span class="math-radicand-body">`)
		writeMathChildHTML(writer, node, 0)
		_, _ = writer.WriteString(`</span></span></span>`)
	case mathScripts:
		_, _ = writer.WriteString(`<span class="math-scripts"><span class="math-base">`)
		writeMathChildHTML(writer, node, 0)
		_, _ = writer.WriteString(`</span><span class="math-script-stack">`)
		if len(node.children) > 1 && node.children[1].kind != mathEmpty {
			_, _ = writer.WriteString(`<sup>`)
			writeMathNodeHTML(writer, node.children[1])
			_, _ = writer.WriteString(`</sup>`)
		}
		if len(node.children) > 2 && node.children[2].kind != mathEmpty {
			_, _ = writer.WriteString(`<sub>`)
			writeMathNodeHTML(writer, node.children[2])
			_, _ = writer.WriteString(`</sub>`)
		}
		_, _ = writer.WriteString(`</span></span>`)
	case mathMatrix:
		writeMathMatrixHTML(writer, node)
	case mathStyle:
		_, _ = writer.WriteString(`<span class="math-style math-` + node.name + `">`)
		writeMathChildHTML(writer, node, 0)
		_, _ = writer.WriteString(`</span>`)
	}
}

func writeMathChildHTML(writer util.BufWriter, node mathNode, index int) {
	if index < len(node.children) {
		writeMathNodeHTML(writer, node.children[index])
	}
}

func writeMathTextHTML(writer util.BufWriter, value string) {
	if value != "" {
		_, _ = writer.Write(util.EscapeHTML([]byte(value)))
	}
}

func writeMathMatrixHTML(writer util.BufWriter, node mathNode) {
	left, right := mathEnvironmentDelimiters(node.name)
	_, _ = writer.WriteString(`<span class="math-matrix">`)
	if left != "" {
		_, _ = writer.WriteString(`<span class="math-delimiter">`)
		writeMathTextHTML(writer, left)
		_, _ = writer.WriteString(`</span>`)
	}
	_, _ = writer.WriteString(`<span class="math-grid">`)
	for _, row := range node.rows {
		_, _ = writer.WriteString(`<span class="math-row">`)
		for column, cell := range row {
			alignment := mathCellAlignment(node, column)
			_, _ = writer.WriteString(`<span class="math-cell math-align-` + alignment + `">`)
			writeMathNodeHTML(writer, cell)
			_, _ = writer.WriteString(`</span>`)
		}
		_, _ = writer.WriteString(`</span>`)
	}
	_, _ = writer.WriteString(`</span>`)
	if right != "" {
		_, _ = writer.WriteString(`<span class="math-delimiter">`)
		writeMathTextHTML(writer, right)
		_, _ = writer.WriteString(`</span>`)
	}
	_, _ = writer.WriteString(`</span>`)
}

func mathCellAlignment(node mathNode, column int) string {
	if node.name == "cases" {
		return "left"
	}
	if node.name == "aligned" || strings.HasPrefix(node.name, "align") {
		if column%2 == 0 {
			return "right"
		}
		return "left"
	}
	if column < len(node.alignments) {
		switch node.alignments[column] {
		case mathAlignLeft:
			return "left"
		case mathAlignRight:
			return "right"
		}
	}
	return "center"
}

func mathEnvironmentDelimiters(name string) (string, string) {
	switch name {
	case "pmatrix":
		return "(", ")"
	case "bmatrix":
		return "[", "]"
	case "Bmatrix":
		return "{", "}"
	case "vmatrix":
		return "|", "|"
	case "Vmatrix":
		return "‖", "‖"
	case "cases":
		return "{", ""
	default:
		return "", ""
	}
}
