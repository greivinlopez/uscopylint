package main

import "strings"

func identByte(c byte) bool {
	return alpha(c) || c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 128
}

// Match explicit logging receivers. A function named debug(), or a component
// named DebugPanel, says too little about whether its text is visible.
func (l *lexer) logCall(start, receiverEnd, end int) int {
	receiver := l.s[start:receiverEnd]
	switch receiver {
	case "console", "logger", "log", "logging", "Logger", "LOGGER", "slog":
	default:
		return -1
	}
	i := l.skipSpace(receiverEnd, end)
	if i < end && l.s[i] == '?' {
		i++
	}
	if i == end || l.s[i] != '.' {
		return -1
	}
	i = l.skipSpace(i+1, end)
	a := i
	for i < end && identByte(l.s[i]) {
		i++
	}
	method := l.s[a:i]
	if receiver == "console" {
		if !oneOf(method, "debug log trace info warn error") {
			return -1
		}
	} else if !oneOf(strings.ToLower(method), "debug trace debugf tracef debugcontext") {
		return -1
	}
	i = l.skipSpace(i, end)
	if strings.HasPrefix(l.s[i:end], "?.") {
		i = l.skipSpace(i+2, end)
	}
	if i < end && l.s[i] == '(' {
		return i
	}
	return -1
}

// At the start of a JS expression, a regex can contain quotes, slashes in a
// character class, or parentheses that do not close the surrounding call.
func (l *lexer) regexEnd(start, end int) int {
	inClass := false
	for i := start + 1; i < end; i++ {
		switch l.s[i] {
		case '\r', '\n':
			return start + 1
		case '\\':
			i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				return i + 1
			}
		}
	}
	return start + 1
}
