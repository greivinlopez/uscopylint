package main

import (
	"fmt"
	"io"
	"strings"
)

// Findings arrive in source order. Print the source once, even when several
// spans (such as a JSX attribute and a text node) match on the same line.
func writeDiagnostics(out io.Writer, findings []finding) error {
	for len(findings) > 0 {
		first := findings[0]
		n := 0
		for _, f := range findings {
			if f.Path != first.Path || f.Line != first.Line || f.SourceLine != first.SourceLine ||
				(n > 0 && f.Column < findings[n-1].EndColumn) {
				break
			}
			context := ""
			if p := f.Phrase; p != nil {
				context = fmt.Sprintf(" (%s → %s)", p.Word, p.Suggestion)
			}
			if _, err := fmt.Fprintf(out, "%s:%d:%d: %s → %s%s\n", f.Path, f.Line, f.Column, f.Word, f.Suggestion, context); err != nil {
				return err
			}
			n++
		}
		if _, err := fmt.Fprintf(out, "  | %s\n  | %s\n", first.SourceLine, markWords(first.SourceLine, findings[:n])); err != nil {
			return err
		}
		findings = findings[n:]
	}
	return nil
}

func markWords(source string, matches []finding) string {
	line := []rune(source)
	// CI log viewers differ on the width of emoji, combining marks, and
	// other Unicode text. Inline brackets identify the exact spans there.
	inline := len(line) != len(source)
	var b strings.Builder
	prev := 0
	for _, f := range matches {
		start, end := f.Column-1, f.EndColumn-1
		gap := line[prev:start]
		if inline {
			b.WriteString(string(gap))
			b.WriteString("[[")
			b.WriteString(f.Word)
			b.WriteString("]]")
		} else {
			for _, r := range gap {
				if r == '\t' {
					b.WriteByte('\t')
				} else {
					b.WriteByte(' ')
				}
			}
			for _, r := range line[start:end] {
				if r == '\t' {
					b.WriteByte('\t')
				} else {
					b.WriteByte('^')
				}
			}
		}
		prev = end
	}
	if inline {
		b.WriteString(string(line[prev:]))
	}
	return b.String()
}
