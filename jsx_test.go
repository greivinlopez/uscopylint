package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSXEditsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, src, fixed string
		edits            int
	}{
		{"inline", "<label>Shopping <b>trolley</b></label>", "<label>Shopping <b>cart</b></label>", 1},
		{"nested and independent attribute", "<>💳 Cancelled <b title='Colour'><i>cheque</i></b>!</>", "<>💳 Canceled <b title='Color'><i>check</i></b>!</>", 3},
		{"multiline", "<label>\r\n  <span>Shopping</span>{ ' ' }\r\n  <em>trolley</em>\r\n</label>", "<label>\r\n  <span>Shopping</span>{ ' ' }\r\n  <em>cart</em>\r\n</label>", 1},
		{"literal children and comments", "<p>{'Shopping '}{/* colour */}<b>{\"trolley\"}</b></p>", "<p>{'Shopping '}{/* colour */}<b>{\"cart\"}</b></p>", 1},
		{"inside one word", "<p>💳 Shopping tro<b>lley</b>!</p>", "<p>💳 Shopping car<b>t</b>!</p>", 2},
		{"contraction", "<p>cash{' '}<b>machine</b> nearby</p>", "<p>ATM{''}<b></b> nearby</p>", 3},
		{"expansion", "<p>Billing <em>postcode</em></p>", "<p>Billing <em>postal code</em></p>", 1},
		{"separate labels", "<><label>Shopping </label><label>trolley</label></>", "", 0},
		{"dynamic child", "<p>Shopping {count}<b>trolley</b></p>", "", 0},
		{"component", "<p>Shopping <Badge>trolley</Badge></p>", "", 0},
		{"missing rendered space", "<p>Shopping\n  <b>trolley</b></p>", "", 0},
		{"whole word boundaries", "<p>Shopping <b>trolley</b>bus</p>", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, diag bytes.Buffer
			args := []string{"--output", "JSON", "--stdin-name", "ui.tsx", "-"}
			code := run(args, strings.NewReader(tc.src), &out, &diag)
			var got []finding
			if err := json.Unmarshal(out.Bytes(), &got); err != nil || diag.Len() != 0 || len(got) != tc.edits {
				t.Fatalf("got %s (err %v, stderr %s)", out.String(), err, diag.String())
			}
			if tc.edits == 0 {
				if code != 0 {
					t.Fatalf("exit %d", code)
				}
				return
			}
			if code != 1 {
				t.Fatalf("exit %d", code)
			}
			if tc.name == "inline" {
				f := got[0]
				want := phraseContext{"Shopping trolley", "Shopping cart", 1, 8}
				if f.Path != "ui.tsx" || f.Line != 1 || f.Column != 20 || f.EndColumn != 27 || f.Phrase == nil || *f.Phrase != want {
					t.Fatalf("unexpected JSON: %s", out.String())
				}
			}
			lines := strings.Split(tc.src, "\n")
			original := append([]string(nil), lines...)
			for i := len(got) - 1; i >= 0; i-- {
				f := got[i]
				line := []rune(lines[f.Line-1])
				if f.SourceLine != strings.TrimSuffix(original[f.Line-1], "\r") || string(line[f.Column-1:f.EndColumn-1]) != f.Word {
					t.Fatalf("invalid source range %+v", f)
				}
				lines[f.Line-1] = string(line[:f.Column-1]) + f.Suggestion + string(line[f.EndColumn-1:])
			}
			fixed := strings.Join(lines, "\n")
			if fixed != tc.fixed {
				t.Fatalf("got %q, want %q", fixed, tc.fixed)
			}
			out.Reset()
			if code := run(args, strings.NewReader(fixed), &out, &diag); code != 0 || strings.TrimSpace(out.String()) != "[]" {
				t.Fatalf("rescan exited %d: %s %s", code, out.String(), diag.String())
			}
		})
	}
}

func TestLogCallBoundary(t *testing.T) {
	for _, tc := range []struct{ file, src, want string }{
		{"ui.tsx", "console.debug(\n  format(')','Colour'), /* ) */ /[(]/, /[)]/, `Cheque memo ${id}`\n); toast('Favourite');", "Favourite"},
		{"ui.js", "logger?.debug?.('Colour'); console.log('Centre'); console.warn('Grey'); dialog.debug('Customise');", "Customise"},
		{"ui.ts", "console.debug(a / b, 'Favourite'); show('Cheque memo');", "Cheque memo"},
		{"ui.jsx", "<button onClick={() => {console.debug('Colour'); toast('Centre')}}>Favourite</button>", "Centre|Favourite"},
		{"ui.py", "logging.debug('Colour %s', f(')'))\nshow('Customise')", "Customise"},
		{"ui.go", "slog.DebugContext(ctx, \"Cheque memo\"); logger.Debugf(\"Colour %s\", id); label(\"Favourite\")", "Favourite"},
		{"ui.js", "show('console.debug(Colour)'); myconsole.debug('Centre'); debug('Grey');", "Colour|Centre|Grey"},
		{"ui.txt", "console.debug('Cheque memo')", "Cheque memo"},
	} {
		var got []string
		scan(tc.file, tc.src, "auto", dictionary(), nil, func(f finding) { got = append(got, f.Word) })
		if strings.Join(got, "|") != tc.want {
			t.Errorf("%s: got %v, want %s", tc.src, got, tc.want)
		}
	}
}

func TestSplitPhraseChangedLines(t *testing.T) {
	src := "<p><span>Shopping </span>\n<b>trolley</b></p>\nconsole.debug('Colour')"
	for _, tc := range []struct{ changed, want int }{{1, 1}, {2, 1}, {3, 0}} {
		var got []finding
		scan("app.tsx", src, "auto", dictionary(), []lineRange{{tc.changed, tc.changed}}, func(f finding) { got = append(got, f) })
		if len(got) != tc.want {
			t.Fatalf("line %d: %+v", tc.changed, got)
		}
		if len(got) > 0 && (got[0].Line != 2 || got[0].Phrase.Line != 1 || got[0].Word != "trolley") {
			t.Fatalf("%+v", got[0])
		}
	}
}

func TestBundledPhrasesAcrossInlineTags(t *testing.T) {
	v := dictionary()
	for _, row := range strings.Split(phrases, "\n") {
		key, us, ok := strings.Cut(row, "|")
		if !ok || strings.HasPrefix(row, "#") {
			continue
		}
		key, us = strings.TrimSpace(key), strings.TrimSpace(us)
		for i := 1; i < len(key); i++ {
			src := "<p>" + key[:i] + "<b>" + key[i:] + "</b></p>"
			var got []finding
			scan("x.jsx", src, "auto", v, nil, func(f finding) { got = append(got, f) })
			fixed := []rune(src)
			for j := len(got) - 1; j >= 0; j-- {
				f := got[j]
				if f.Phrase == nil || f.Phrase.Word != key || f.Phrase.Suggestion != us || string(fixed[f.Column-1:f.EndColumn-1]) != f.Word {
					t.Fatalf("%s: %+v", src, f)
				}
				fixed = []rune(string(fixed[:f.Column-1]) + f.Suggestion + string(fixed[f.EndColumn-1:]))
			}
			rendered := strings.NewReplacer("<p>", "", "</p>", "", "<b>", "", "</b>", "").Replace(string(fixed))
			if rendered != us {
				t.Fatalf("%s: %q, want %q", src, rendered, us)
			}
		}
	}
}
