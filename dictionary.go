package main

import (
	_ "embed"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed spellings.txt
var coreSpellings string

//go:embed spellings-codespell.txt
var codespellSpellings string

var spellings = coreSpellings + "\n" + codespellSpellings

//go:embed phrases.txt
var phrases string

// Each edge represents a whole word or a separator. A terminal can also
// have children, as in "cheque", "cheque memo", and "cheque memo line".
type termNode struct {
	next        map[string]*termNode
	terminal    bool
	replacement string
}
type vocabulary struct {
	root         termNode
	allowedWords map[string]bool
}

func dictionary() *vocabulary {
	v := &vocabulary{allowedWords: make(map[string]bool)}
	for _, line := range strings.Split(spellings, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		for _, suffix := range strings.Split(fields[2], ",") {
			if suffix == "-" {
				suffix = ""
			}
			v.add(fields[0]+suffix, fields[1]+suffix)
		}
	}
	if err := v.loadTerms(phrases); err != nil {
		panic(err)
	} // Embedded data is validated by tests.
	return v
}

func wordEnd(s string, start int) int {
	i := start
	for i < len(s) {
		r, n := utf8.DecodeRuneInString(s[i:])
		if !wordRune(r) {
			break
		}
		i += n
	}
	return i
}

func nextTermToken(s string, i int) (string, int) {
	r, n := utf8.DecodeRuneInString(s[i:])
	switch {
	case wordRune(r):
		end := wordEnd(s, i)
		return strings.ToLower(s[i:end]), end
	case r == '\t' || unicode.Is(unicode.Zs, r):
		i += n
		for i < len(s) {
			r, n = utf8.DecodeRuneInString(s[i:])
			if r != '\t' && !unicode.Is(unicode.Zs, r) {
				break
			}
			i += n
		}
		return " ", i
	case r == '-' || r == '‐' || r == '‑':
		return "-", i + n
	case r == '\'' || r == '’':
		return "'", i + n
	default:
		return "", i + n
	}
}

func (v *vocabulary) add(key, repl string) {
	node := &v.root
	for i := 0; i < len(key); {
		token, end := nextTermToken(key, i)
		if token == "" {
			return
		}
		if node.next == nil {
			node.next = make(map[string]*termNode)
		}
		if node.next[token] == nil {
			node.next[token] = &termNode{}
		}
		node = node.next[token]
		i = end
	}
	node.terminal = true
	node.replacement = repl
}

// Entries can override the embedded preference. Validate the complete key so
// punctuation in a configuration file cannot silently broaden a match.
func (v *vocabulary) loadTerms(data string) error {
	for n, row := range strings.Split(strings.TrimPrefix(data, "\uFEFF"), "\n") {
		row = strings.TrimSpace(row)
		if row == "" || strings.HasPrefix(row, "#") {
			continue
		}
		key, repl, ok := strings.Cut(row, "|")
		key = strings.TrimSpace(key)
		repl = strings.TrimSpace(repl)
		if !ok || key == "" || repl == "" || strings.Contains(repl, "|") {
			return fmt.Errorf("line %d: expected UK wording | US wording", n+1)
		}
		first, _ := utf8.DecodeRuneInString(key)
		last, _ := utf8.DecodeLastRuneInString(key)
		if !wordRune(first) || !wordRune(last) {
			return fmt.Errorf("line %d: wording must begin and end with a word", n+1)
		}
		for i := 0; i < len(key); {
			token, end := nextTermToken(key, i)
			if token == "" {
				return fmt.Errorf("line %d: unsupported separator in %q", n+1, key)
			}
			i = end
		}
		v.add(key, repl)
	}
	return nil
}

func (v *vocabulary) allow(key string) {
	if key == "" {
		return
	}
	if wordEnd(key, 0) == len(key) {
		v.allowedWords[strings.ToLower(key)] = true
		return
	}
	v.add(key, "")
}

func (v *vocabulary) match(s string, firstEnd int) (end int, replacement string) {
	first := strings.ToLower(s[:firstEnd])
	node := v.root.next[first]
	if node == nil || v.allowedWords[first] {
		return firstEnd, ""
	}
	end = firstEnd
	for i := firstEnd; ; {
		if node.terminal {
			end, replacement = i, node.replacement
		}
		if i == len(s) || node.next == nil {
			break
		}
		token, next := nextTermToken(s, i)
		node = node.next[token]
		if node == nil || v.allowedWords[token] {
			break
		}
		i = next
	}
	if replacement == "" {
		return end, ""
	}
	replacement = caseSuggestion(s[:end], replacement)
	if replacement == s[:end] {
		return end, ""
	}
	return end, replacement
}

func initialUpper(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func caseSuggestion(src, repl string) string {
	if src == strings.ToUpper(src) {
		return strings.ToUpper(repl)
	}
	if src == strings.ToLower(src) {
		return repl
	}
	words := strings.Fields(src)
	if len(words) == 1 && src != initialUpper(strings.ToLower(src)) {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(src)
	if !unicode.IsUpper(first) {
		return repl
	}
	if len(words) > 1 {
		title := true
		for _, w := range words {
			r, _ := utf8.DecodeRuneInString(w)
			if !unicode.IsUpper(r) {
				title = false
				break
			}
		}
		if title {
			out := strings.Fields(repl)
			for i, w := range out {
				out[i] = initialUpper(w)
			}
			return strings.Join(out, " ")
		}
	}
	return initialUpper(repl)
}
