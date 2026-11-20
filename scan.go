package main

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type finding struct {
	Path       string         `json:"path"`
	Line       int            `json:"line"`
	Column     int            `json:"column"`
	Word       string         `json:"word"`
	Suggestion string         `json:"suggestion"`
	SourceLine string         `json:"sourceLine"`
	Phrase     *phraseContext `json:"phrase,omitempty"`
	EndColumn  int            `json:"endColumn"` // Exclusive, like a Go slice bound.
}
type lineRange struct{ first, last int }
type lexer struct {
	s, ext                 string
	emit                   func(int, int)
	emitRun                func([]textPiece)
	quiet                  bool
	slash, hash, dash, jsx bool
}

func oneOf(s, fields string) bool {
	return s != "" && !strings.ContainsRune(s, ' ') && strings.Contains(" "+fields+" ", " "+s+" ")
}
func scan(path, s, mode string, words *vocabulary, ranges []lineRange, report func(finding)) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	l := lexer{s: s, ext: ext}
	l.hash = oneOf(ext, "py pyw rb sh bash zsh fish pl pm r jl nim ps1 php toml ex exs") || strings.HasPrefix(s, "#!")
	l.dash = oneOf(ext, "sql lua hs elm")
	l.slash = !oneOf(ext, "py pyw rb sh bash zsh fish pl pm r jl nim ps1 ex exs toml sql lua hs elm")
	l.jsx = oneOf(ext, "jsx tsx js mjs cjs")
	var edits []sourceEdit
	l.emit = func(a, b int) {
		words.matches(s[a:b], func(start, end int, repl string) {
			edits = append(edits, sourceEdit{start: a + start, end: a + end, repl: repl})
		})
	}
	l.emitRun = func(pieces []textPiece) { edits = append(edits, jsxEdits(s, pieces, words)...) }
	if mode == "auto" {
		switch {
		case oneOf(ext, "html htm xml xhtml svg vue svelte astro gohtml tmpl hbs handlebars erb ejs jinja jinja2 j2 twig"),
			ext == "php" && strings.Contains(s, "<?"):
			mode = "markup"
		case oneOf(ext, "md markdown mdx"):
			mode = "markdown"
		case oneOf(ext, "yaml yml"):
			mode = "yaml"
		case l.hash || l.dash || l.jsx || oneOf(ext, "go rs js ts mts cts java kt kts c h cc hh cpp hpp cxx cs fs fsx swift dart scala groovy json jsonc css scss less m mm ex exs zig sol protobuf proto clj cljs vb pas"):
			mode = "code"
		default:
			mode = "text"
		}
	}
	switch mode {
	case "code":
		l.code(0, len(s), 0, 0)
	case "markup":
		l.markup(0, len(s), false, 0)
	case "markdown":
		l.markdown()
	case "yaml":
		l.yaml()
	default:
		l.emit(0, len(s))
	}
	if len(edits) == 0 {
		return
	}
	// Attribute diagnostics can arrive before the enclosing JSX text run.
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	reportEdits(path, s, edits, ranges, report)
}
func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}
func white(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func alpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func nameByte(c byte) bool {
	return alpha(c) || (c >= '0' && c <= '9') || strings.ContainsRune("-_:.@", rune(c))
}
func (l *lexer) skipSpace(i, end int) int {
	for i < end && white(l.s[i]) {
		i++
	}
	return i
}
func (l *lexer) after(i, end int, delim string) int {
	if k := strings.Index(l.s[i:end], delim); k >= 0 {
		return i + k + len(delim)
	}
	return end
}

// A nonzero stop delimiter ends a template expression or logging call.
func (l *lexer) code(i, end int, stop byte, depth int) int {
	if depth > 128 {
		return end
	}
	nested := 0
	js := l.jsx || oneOf(l.ext, "ts mts cts html htm vue svelte astro")
	canStartRegex := true
	for i < end {
		s := l.s[i:end]
		c := s[0]
		nextCanStartRegex := js && strings.ContainsRune("([{:;,=!?&|>+*-^~%<", rune(c))
		switch {
		case white(c):
			i++
			continue
		case l.ext == "lua" && strings.HasPrefix(s, "--[["):
			i = l.after(i+4, end, "]]")
			continue
		case l.slash && strings.HasPrefix(s, "//"), l.hash && c == '#', l.dash && strings.HasPrefix(s, "--"),
			l.ext == "vb" && c == '\'', (l.ext == "clj" || l.ext == "cljs") && c == ';':
			i = l.after(i, end, "\n")
			continue
		case (l.slash || l.ext == "sql") && strings.HasPrefix(s, "/*"):
			i = l.after(i+2, end, "*/")
			continue
		case js && c == '/':
			i++
			nextCanStartRegex = true // Division can be followed by a regex.
			if canStartRegex {
				after := l.regexEnd(i-1, end)
				nextCanStartRegex = after == i
				i = after
			}
		case l.jsx && c == '<' && i+1 < end && (alpha(s[1]) || s[1] == '>') && l.jsxStart(i) && !looksGeneric(s):
			i = l.markup(i, end, true, depth+1)
		case l.ext == "rs" && c == '\'' && len(s) > 1 && (alpha(s[1]) || s[1] == '_') && (len(s) < 3 || s[2] != '\''):
			i++ // Rust lifetime or loop label.
		case alpha(c) || c == '_' || c == '$':
			start := i
			for i < end && identByte(l.s[i]) {
				i++
			}
			nextCanStartRegex = js && oneOf(l.s[start:i], "return throw case yield await void typeof")
			if !l.quiet && (start == 0 || !identByte(l.s[start-1])) {
				if open := l.logCall(start, i, end); open >= 0 {
					silent := *l
					silent.quiet = true
					silent.emit = func(int, int) {}
					silent.emitRun = func([]textPiece) {}
					i = silent.code(open+1, end, ')', depth+1)
				}
			}
		case c == '"' || c == '\'' || c == 96:
			i = l.literal(i, end, depth)
		case stop == ')' && c == '(' || stop == '}' && c == '{':
			nested++
			i++
		case stop != 0 && c == stop:
			i++
			if nested == 0 {
				return i
			}
			nested--
		default:
			i++
		}
		canStartRegex = nextCanStartRegex
	}
	return end
}
func (l *lexer) jsxStart(i int) bool {
	j := i - 1
	for j >= 0 && white(l.s[j]) {
		j--
	}
	if j < 0 || strings.ContainsRune("(=[:>{,;!&|?", rune(l.s[j])) {
		return true
	}
	return j >= 5 && l.s[j-5:j+1] == "return"
}
func looksGeneric(s string) bool {
	i := 1
	for i < len(s) && nameByte(s[i]) {
		i++
	}
	return i < len(s) && s[i] == ','
}
func (l *lexer) literal(i, end, depth int) int {
	start := i
	c := l.s[i]
	delim := l.s[i : i+1]
	if i+3 <= end && l.s[i+1] == c && l.s[i+2] == c && oneOf(l.ext, "py pyw kt kts java scala cs swift groovy dart jl ex exs") {
		delim = l.s[i : i+3]
	}
	i += len(delim)
	chunk := i
	dollarBraces := c == 96 && oneOf(l.ext, "js jsx ts tsx mts cts mjs cjs html htm vue svelte astro") || oneOf(l.ext, "kt kts dart")
	braces := start > 0 && ((oneOf(l.ext, "py pyw") && (l.s[start-1] == 'f' || l.s[start-1] == 'F')) || (l.ext == "cs" && l.s[start-1] == '$'))
	for i < end {
		switch {
		case l.s[i] == '\n' && len(delim) == 1 && c != 96 && oneOf(l.ext, "js ts jsx tsx mts mjs cjs cts json jsonc java c cpp cs go py pyw"):
			l.literalText(chunk, i)
			return i + 1
		case strings.HasPrefix(l.s[i:end], delim):
			next := i + len(delim)
			j := l.skipSpace(next, end)
			k := start - 1
			for k >= 0 && white(l.s[k]) {
				k--
			}
			key := j < end && l.s[j] == ':' && (k < 0 || l.s[k] == '{' || l.s[k] == ',')
			if !key {
				l.literalText(chunk, i)
			}
			return next
		case l.s[i] == '\\' && i+1 < end && strings.ContainsRune("nrt", rune(l.s[i+1])) && !(c == 96 && l.ext == "go"):
			l.literalText(chunk, i)
			i += 2
			chunk = i
		case l.s[i] == '\\' && !(c == 96 && l.ext == "go"):
			i++
			if i < end {
				i++
			}
		case braces && strings.HasPrefix(l.s[i:end], "{{"):
			i += 2
		case braces && l.s[i] == '{':
			l.literalText(chunk, i)
			i = l.code(i+1, end, '}', depth+1)
			chunk = i
		case dollarBraces && strings.HasPrefix(l.s[i:end], "${"):
			l.literalText(chunk, i)
			i = l.code(i+2, end, '}', depth+1)
			chunk = i
		default:
			i++
		}
	}
	l.literalText(chunk, end)
	return end
}
func (l *lexer) literalText(a, b int) {
	s := strings.TrimSpace(l.s[a:b])
	if !strings.ContainsAny(s, " \t\r\n") && (strings.Contains(s, "/") || strings.HasPrefix(s, "#") || strings.HasPrefix(s, ".")) {
		return
	}
	l.emit(a, b)
}
func (l *lexer) markup(i, end int, single bool, depth int) int {
	if depth > 128 {
		return end
	}
	level := 0
	var pieces []textPiece
	flush := func() {
		if len(pieces) > 0 {
			l.emitRun(pieces)
			pieces = pieces[:0]
		}
	}
	defer flush()
	for i < end {
		s := l.s[i:end]
		switch {
		case strings.HasPrefix(s, "<!--"):
			i = l.after(i+4, end, "-->")
		case strings.HasPrefix(s, "{#"):
			i = l.after(i+2, end, "#}")
		case strings.HasPrefix(s, "{{!"):
			i = l.after(i+3, end, "}}")
		case strings.HasPrefix(s, "<%#"):
			i = l.after(i+3, end, "%>")
		case strings.HasPrefix(s, "<%"):
			i = l.template(i+2, end, "%>", depth)
		case l.ext == "php" && strings.HasPrefix(s, "<?"):
			i = l.template(i+2, end, "?>", depth)
		case strings.HasPrefix(s, "{%"):
			i = l.template(i+2, end, "%}", depth)
		case strings.HasPrefix(s, "{{") && !single:
			i = l.template(i+2, end, "}}", depth)
		case strings.HasPrefix(s, "<!") || strings.HasPrefix(s, "<?"):
			i = l.after(i+2, end, ">")
		case s[0] == '<' && len(s) > 1 && (alpha(s[1]) || s[1] == '/' || s[1] == '>'):
			closing := s[1] == '/'
			j := i + 1
			if closing {
				j++
			}
			a := j
			for j < end && nameByte(l.s[j]) {
				j++
			}
			name := l.s[a:j]
			if single && name != "" && !oneOf(name, "abbr b bdi bdo cite code data del dfn em i ins kbd mark s samp small span strong sub sup time u var") {
				flush()
			}
			tag := strings.ToLower(name)
			var self bool
			i, self = l.tag(j, end, depth)
			if closing {
				level--
			} else if !self && (single || !oneOf(tag, "area base br col embed hr img input link meta param source track wbr")) {
				level++
				if !single && (tag == "style" || tag == "script") {
					// Find the raw-text end without changing offsets in Unicode text.
					k := i
					for k < end {
						p := strings.Index(l.s[k:end], "</")
						if p < 0 {
							k = end
							break
						}
						k += p
						e := k + 2 + len(tag)
						if e < end && strings.EqualFold(l.s[k+2:e], tag) && (l.s[e] == '>' || white(l.s[e])) {
							break
						}
						k += 2
					}
					if tag == "script" {
						l.code(i, k, 0, depth+1)
					}
					i = k
				}
			}
			if single && level <= 0 {
				return i
			}
		case s[0] == '{' && (single || oneOf(l.ext, "svelte vue astro")):
			if single {
				if next, piece, ok := l.staticChild(i, end); ok {
					if piece.end > piece.start {
						pieces = append(pieces, piece)
					}
					i = next
					continue
				}
			}
			flush()
			i = l.code(i+1, end, '}', depth+1)
		default:
			a := i
			i++
			for i < end && l.s[i] != '<' && l.s[i] != '{' {
				i++
			}
			if single {
				jsxText(l.s, a, i, func(start, end int) { pieces = append(pieces, textPiece{start: start, end: end}) }, flush)
			} else {
				l.emit(a, i)
			}
		}
	}
	return end
}
func (l *lexer) template(i, end int, delim string, depth int) int {
	n := strings.Index(l.s[i:end], delim)
	if n < 0 {
		l.code(i, end, 0, depth+1)
		return end
	}
	l.code(i, i+n, 0, depth+1)
	return i + n + len(delim)
}

func (l *lexer) tag(i, end, depth int) (int, bool) {
	for i < end {
		i = l.skipSpace(i, end)
		if i >= end {
			break
		}
		if l.s[i] == '>' {
			return i + 1, i > 0 && l.s[i-1] == '/'
		}
		if l.s[i] == '{' {
			i = l.code(i+1, end, '}', depth+1)
			continue
		}
		a := i
		for i < end && nameByte(l.s[i]) {
			i++
		}
		if a == i {
			i++
			continue
		}
		attr := strings.ToLower(l.s[a:i])
		i = l.skipSpace(i, end)
		if i >= end || l.s[i] != '=' {
			continue
		}
		i = l.skipSpace(i+1, end)
		if i >= end {
			break
		}
		binding := strings.HasPrefix(attr, ":") || strings.HasPrefix(attr, "v-bind:")
		attr = strings.TrimPrefix(strings.TrimPrefix(attr, "v-bind:"), ":")
		visible := copyAttribute(attr)
		c := l.s[i]
		if c == '{' {
			if visible || strings.HasPrefix(attr, "on") {
				i = l.code(i+1, end, '}', depth+1)
			} else {
				i = skipExpression(l.s, i, end)
			}
			continue
		}
		if c == '"' || c == '\'' {
			a = i + 1
			k := strings.IndexByte(l.s[a:end], c)
			if k < 0 {
				i = end
			} else {
				i = a + k
			}
			if visible {
				if binding {
					l.code(a, i, 0, depth+1)
				} else {
					l.emit(a, i)
				}
			}
			if i < end {
				i++
			}
		} else {
			a = i
			for i < end && !white(l.s[i]) && l.s[i] != '>' {
				i++
			}
			if visible {
				l.emit(a, i)
			}
		}
	}
	return end, false
}
func copyAttribute(attr string) bool {
	if oneOf(attr, "alt value summary header children aria-roledescription aria-valuetext") {
		return true
	}
	for _, suffix := range []string{"text", "label", "title", "placeholder", "description", "message", "hint", "heading", "caption", "tooltip"} {
		if strings.HasSuffix(attr, suffix) {
			return true
		}
	}
	return false
}

func skipExpression(s string, i, end int) int {
	level := 1
	var quote byte
	for i++; i < end; i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', 96:
			quote = c
		case '{':
			level++
		case '}':
			level--
			if level == 0 {
				return i + 1
			}
		}
	}
	return end
}
func (l *lexer) markdown() {
	fence := ""
	for i := 0; i < len(l.s); {
		end := l.after(i, len(l.s), "\n")
		row := l.s[i:end]
		trim := strings.TrimSpace(row)
		switch {
		case fence != "":
			if strings.HasPrefix(trim, fence) && strings.Trim(trim, fence[:1]) == "" {
				fence = ""
			}
		case strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~"):
			n := 0
			for n < len(trim) && trim[n] == trim[0] {
				n++
			}
			fence = trim[:n]
		default:
			for a := i; a < end; {
				k := strings.IndexByte(l.s[a:end], 96)
				if k < 0 {
					l.emit(a, end)
					break
				}
				k += a
				l.emit(a, k)
				j := k + 1
				for j < end && l.s[j] == 96 {
					j++
				}
				a = l.after(j, end, l.s[k:j])
			}
		}
		i = end
	}
}
func (l *lexer) yaml() {
	for i := 0; i < len(l.s); {
		end := l.after(i, len(l.s), "\n")
		a := i
		var quote byte
		for j := i; j < end; j++ {
			c := l.s[j]
			if quote != 0 {
				if c == '\\' && quote == '"' {
					j++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			if (c == '"' || c == '\'') && (j == i || white(l.s[j-1])) {
				quote = c
				continue
			}
			if c == '#' && (j == i || white(l.s[j-1])) {
				break
			}
			if c == ':' && (j+1 == end || white(l.s[j+1])) {
				a = j + 1
				break
			}
		}
		quote = 0
		for j := a; j < end; j++ {
			c := l.s[j]
			if quote != 0 {
				if c == '\\' && quote == '"' {
					j++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			if (c == '"' || c == '\'') && (j == a || white(l.s[j-1])) {
				quote = c
				continue
			}
			if c == '#' && (j == a || white(l.s[j-1])) {
				l.emit(a, j)
				a = end
				break
			}
		}
		l.emit(a, end)
		i = end
	}
}
