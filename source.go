package main

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Keep the original edit contract: every finding replaces one exact slice of
// one source line. Several edits can belong to one rendered phrase.
type phraseContext struct {
	Word       string `json:"word"`
	Suggestion string `json:"suggestion"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
}
type sourceEdit struct {
	start, end int
	repl       string
	group      *jsxMatch
}
type jsxMatch struct {
	start, end int // bounds of all contributing source text, including unchanged words
	context    phraseContext
}

func (v *vocabulary) matches(text string, report func(int, int, string)) {
	for i := 0; i < len(text); {
		r, n := utf8.DecodeRuneInString(text[i:])
		if !wordRune(r) {
			i += n
			continue
		}
		start := i
		n, repl := v.match(text[start:], wordEnd(text, start)-start)
		i += n
		if repl != "" {
			report(start, i, repl)
		}
	}
}

func reportEdits(path, s string, edits []sourceEdit, ranges []lineRange, report func(finding)) {
	starts := []int{0}
	for i := strings.IndexByte(s, '\n'); i >= 0; {
		starts = append(starts, i+1)
		next := strings.IndexByte(s[i+1:], '\n')
		if next < 0 {
			break
		}
		i += next + 1
	}
	lineOf := func(off int) int { return sort.Search(len(starts), func(i int) bool { return starts[i] > off }) }
	// Cache the current line and column for repeated hits on long source lines.
	lastLine, pos, col := 0, 0, 1
	var source string
	for _, e := range edits {
		line := lineOf(e.start)
		var context *phraseContext
		first, last := line, line
		if g := e.group; g != nil {
			context = &g.context
			if context.Line == 0 {
				context.Line = lineOf(g.start)
				context.Column = 1 + utf8.RuneCountInString(s[starts[context.Line-1]:g.start])
			}
			first, last = context.Line, lineOf(g.end-1)
		}
		if ranges != nil {
			i := sort.Search(len(ranges), func(i int) bool { return ranges[i].last >= first })
			if i == len(ranges) || ranges[i].first > last {
				continue
			}
		}
		lineStart := starts[line-1]
		if line != lastLine {
			end := len(s)
			if line < len(starts) {
				end = starts[line] - 1
				if end > lineStart && s[end-1] == '\r' {
					end--
				}
			}
			source = strings.Clone(s[lineStart:end])
			pos, col, lastLine = lineStart, 1, line
		}
		col += utf8.RuneCountInString(s[pos:e.start])
		pos = e.start
		word := source[e.start-lineStart : e.end-lineStart]
		report(finding{Path: path, Line: line, Column: col, EndColumn: col + utf8.RuneCountInString(word), Word: word, Suggestion: e.repl, SourceLine: source, Phrase: context})
	}
}
