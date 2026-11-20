package main

import (
	"sort"
	"strings"
	"unicode/utf8"
)

type textPiece struct {
	start, end int
	quote      byte // Nonzero in a literal JSX child expression.
}

// A literal child has a known value. Leave escape sequences and expressions
// containing operators to the ordinary code scanner.
func (l *lexer) staticChild(open, end int) (int, textPiece, bool) {
	i := l.skipSpace(open+1, end)
	var p textPiece
	for strings.HasPrefix(l.s[i:end], "/*") {
		i = l.skipSpace(l.after(i+2, end, "*/"), end)
	}
	if i < end && (l.s[i] == '\'' || l.s[i] == '"') {
		q := l.s[i]
		a := i + 1
		i = a
		for i < end && l.s[i] != q {
			if l.s[i] == '\\' || l.s[i] == '\r' || l.s[i] == '\n' {
				return 0, p, false
			}
			i++
		}
		if i == end {
			return 0, p, false
		}
		p = textPiece{start: a, end: i, quote: q}
		i = l.skipSpace(i+1, end)
	}
	return i + 1, p, i < end && l.s[i] == '}'
}

func (p textPiece) encode(s string) string {
	if p.quote == 0 {
		return jsxEscaper.Replace(s)
	}
	q := string(p.quote)
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), q, `\`+q)
}

// JSX removes indentation and blank lines at text-node boundaries. Preserve
// explicit spaces. Inside a multiline text node, keep the existing separate
// line checks where JSX would synthesize a space without a literal source span.
func jsxText(s string, a, b int, add func(int, int), boundary func()) {
	first := true
	haveText := false
	for a < b {
		end := a + strings.IndexAny(s[a:b], "\r\n")
		last := end < a
		if last {
			end = b
		}
		start, stop := a, end
		if !first {
			for start < stop && (s[start] == ' ' || s[start] == '\t') {
				start++
			}
		}
		if !last {
			for stop > start && (s[stop-1] == ' ' || s[stop-1] == '\t') {
				stop--
			}
		}
		if start < stop {
			if haveText {
				boundary()
			}
			add(start, stop)
			haveText = true
		}
		if last {
			break
		}
		a = end + 1
		if s[end] == '\r' && a < b && s[a] == '\n' {
			a++
		}
		first = false
	}
}

func jsxEdits(s string, pieces []textPiece, words *vocabulary) []sourceEdit {
	var text strings.Builder
	ends := make([]int, len(pieces))
	for i, p := range pieces {
		text.WriteString(s[p.start:p.end])
		ends[i] = text.Len()
	}
	visible := text.String()
	var edits []sourceEdit
	words.matches(visible, func(start, end int, repl string) {
		first := sort.SearchInts(ends, start+1)
		last := sort.SearchInts(ends, end)
		offset := func(i, v int) int { return pieces[i].end - (ends[i] - v) }
		a, b := offset(first, start), offset(last, end)
		if first == last {
			edits = append(edits, sourceEdit{start: a, end: b, repl: pieces[first].encode(repl)})
			return
		}
		g := &jsxMatch{start: a, end: b, context: phraseContext{Word: visible[start:end], Suggestion: repl}}
		// Partition the replacement at source node boundaries. Preserve word
		// alignment and unchanged formatting for equal-word-count rules.
		put := func(lo, hi int, target string) {
			i := sort.SearchInts(ends, lo+1)
			for lo < hi {
				stop := min(hi, ends[i])
				part := target
				if stop < hi {
					// For a word split mid-letter, keep as much of its original
					// node width as possible without splitting a UTF-8 character.
					n := utf8.RuneCountInString(visible[lo:stop])
					k := 0
					for k < len(target) && n > 0 {
						_, size := utf8.DecodeRuneInString(target[k:])
						k += size
						n--
					}
					part = target[:k]
					target = target[k:]
				}
				if visible[lo:stop] != part {
					edits = append(edits, sourceEdit{start: offset(i, lo), end: offset(i, stop), repl: pieces[i].encode(part), group: g})
				}
				i++
				lo = stop
			}
		}
		replacements := strings.Fields(repl)
		original := strings.Fields(visible[start:end])
		cursor := start
		for len(original) > 1 && len(replacements) > 1 {
			// An expansion such as "billing postcode" keeps the prefix in
			// its own node and replaces just the differently worded tail.
			if len(original) != len(replacements) && original[0] != replacements[0] {
				break
			}
			word := original[0]
			cursor += strings.Index(visible[cursor:end], word)
			put(cursor, cursor+len(word), replacements[0])
			cursor += len(word)
			original, replacements = original[1:], replacements[1:]
		}
		if len(original) > 0 && len(replacements) > 0 {
			cursor += strings.Index(visible[cursor:end], original[0])
			if len(original) == len(replacements) {
				put(cursor, end, strings.Join(replacements, " "))
				return
			}
			// Reflow a contraction or expansion in the first remaining
			// node. Only remove literal text from any subsequent nodes.
			firstEnd := min(end, ends[sort.SearchInts(ends, cursor+1)])
			put(cursor, firstEnd, strings.Join(replacements, " "))
			put(firstEnd, end, "")
		}
	})
	return edits
}

// Replacements are source text. Encode characters that would introduce JSX
// syntax or an entity when a project glossary contains punctuation.
var jsxEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "{", "&#123;", "}", "&#125;")
