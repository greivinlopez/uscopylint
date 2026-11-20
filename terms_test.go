package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryBundledTerm(t *testing.T) {
	v := dictionary()
	seen := map[string]bool{}
	verify := func(uk, us string) {
		t.Helper()
		key := strings.ToLower(uk)
		if seen[key] {
			t.Errorf("duplicate rule %q", uk)
		}
		seen[key] = true
		var got []finding
		scan("labels.txt", uk, "text", v, nil, func(f finding) { got = append(got, f) })
		if len(got) != 1 || got[0].Word != uk || got[0].Suggestion != us {
			t.Errorf("%q: want %q, got %+v", uk, us, got)
		}
		scan("labels.txt", us, "text", v, nil, func(f finding) { t.Errorf("US replacement for %q triggers %+v", uk, f) })
	}
	for _, row := range strings.Split(spellings, "\n") {
		if row == "" || strings.HasPrefix(row, "#") {
			continue
		}
		f := strings.Fields(row)
		for _, suffix := range strings.Split(f[2], ",") {
			if suffix == "-" {
				suffix = ""
			}
			verify(f[0]+suffix, f[1]+suffix)
		}
	}
	for _, row := range strings.Split(phrases, "\n") {
		if row == "" || strings.HasPrefix(row, "#") {
			continue
		}
		uk, us, _ := strings.Cut(row, "|")
		verify(strings.TrimSpace(uk), strings.TrimSpace(us))
	}
}

func TestPhraseCLI(t *testing.T) {
	for _, tc := range []struct{ src, ignore, word, repl string }{
		{"<label>Cheque Memo</label>", "", "Cheque Memo", "Check Memo"},
		{"<label className='greyscale'>Greyscale preview</label>", "", "Greyscale", "Grayscale"},
		{"<Field aria-label=\"💳 Organisational account\" />", "", "Organisational", "Organizational"},
		{"show(\"Enter your routeing number\")", "", "routeing", "routing"},
		{"show(\"Doughnuts, dialogue transcripts and saleable goods\")", "", "", ""},
		{"show(\"Open the dialogue box\")", "", "dialogue box", "dialog box"},
		{"show(\"Greyscale preview\")", "greyscale", "", ""},
		{"<Field placeholder=\"Cheque memo line\" />", "", "Cheque memo line", "Check memo line"},
		{"show(\"CHEQUE MEMO\")", "", "CHEQUE MEMO", "CHECK MEMO"},
		{"show(\"🐈 paying‑in\u00a0slip\")", "", "paying‑in\u00a0slip", "deposit slip"},
		{"show(\"💳 traveller’s  cheque\")", "", "traveller’s  cheque", "traveler's check"},
		{"show(\"Card expiry date\")", "", "Card expiry date", "Card expiration date"},
		{"show(\"Cheque memoir\")", "", "Cheque", "Check"},
		{"show(\"Paying-in slippers\")", "", "", ""},
		{"show(\"expiry\", \"date\"); // cash machine", "", "", ""},
		{"show(`shopping ${x} trolley`)", "", "", ""},
		{"<p>cash<strong>machine</strong></p>", "", "", ""},
		{"show(\"trolley, current account, sort code, tick, memo, cash flow\")", "", "", ""},
		{"show(\"cash\\nmachine\")", "", "", ""},
		{"show(`cash\nmachine`)", "", "", ""},
		{"show(\"Cheque memo\")", "cheque memo", "", ""},
		{"show(\"Cheque memo\")", "cheque", "", ""},
	} {
		var out, diag bytes.Buffer
		status := run([]string{"--format", "json", "--stdin-name", "bill.tsx", "--ignore", tc.ignore, "-"}, strings.NewReader(tc.src), &out, &diag)
		var got []finding
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err, diag.String())
		}
		if tc.word == "" {
			if status != 0 || len(got) != 0 {
				t.Errorf("%q: expected clean, got %+v", tc.src, got)
			}
			continue
		}
		if status != 1 || len(got) != 1 {
			t.Errorf("%q: expected one match, got %+v", tc.src, got)
			continue
		}
		f := got[0]
		if f.Word != tc.word || f.Suggestion != tc.repl || f.Line != 1 || f.SourceLine != tc.src || string([]rune(tc.src)[f.Column-1:f.EndColumn-1]) != tc.word {
			t.Errorf("%q: %+v", tc.src, f)
		}
	}
}

func TestProjectTerminology(t *testing.T) {
	dir := t.TempDir()
	terms := filepath.Join(dir, "terms.txt")
	app := filepath.Join(dir, "ui.js")
	if err := os.WriteFile(terms, []byte("# product language\ncheque memo | payment note\nremittance advice | payment notice\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app, []byte("show(\"Cheque memo / remittance advice\")"), 0644); err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	status := run([]string{"--terms", terms, dir}, nil, &out, &diag)
	if status != 1 || strings.Contains(out.String(), terms) || !strings.Contains(out.String(), "Cheque memo → Payment note") || !strings.Contains(out.String(), "remittance advice → payment notice") {
		t.Fatalf("%d %s %s", status, out.String(), diag.String())
	}
	if err := os.WriteFile(terms, []byte("# comment\ninvalid entry"), 0644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"--terms", terms, dir}, nil, &out, &diag); code != 2 || !strings.Contains(diag.String(), "line 2") {
		t.Fatalf("%d %s", code, diag.String())
	}
}
