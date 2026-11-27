package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCopyContexts(t *testing.T) {
	for _, tc := range []struct{ file, src, want string }{
		{"app.tsx", "const colour = () => <button className='colour' aria-label='Favourite colour' onClick={() => alert(\"Cancelled\")}>Customise {colour}</button>", "Favourite colour Cancelled Customise"},
		{"profile.html", "<!-- Favourite --><style>.colour{}</style><input name='colour' placeholder='Post your favourite colour'><p>We're organised.</p><script>let colour='Catalogue';</script>", "favourite colour organised Catalogue"},
		{"menu.vue", "<span :title=\"colour\">{{ colour }}</span><span :title=\"chosen ? 'Favourites' : 'Colours'\">Customise</span>", "Favourites Colours Customise"},
		{"screen.php", "<button><?= $colour ? 'Customise' : 'Favourite' ?></button><p>New colour</p>", "Customise Favourite colour"},
		{"page.jinja", "{# colour #}<p>{{ colour }} {{ 'Favourite' }} colour</p>", "Favourite colour"},
		{"view.ts", "const colour = `Your ${colour ?? 'favourite'} colour`; // 'cancelled'\nlet x='/favourite/colour';", "favourite colour"},
		{"screen.py", "# 'favourite'\ncolour = f\"\"\"Select\na colour for {favourite}.\"\"\"", "colour"},
		{"view.rs", "fn title<'a>(colour: &'a str) { show(\"Favourite colour\") }", "Favourite colour"},
		{"main.go", "/* \"colour\" */\nfmt.Print(\"Customisation\\nCentre\")", "Customisation Centre"},
		{"config.json", "{\"colour\":\"Favourite\",\"x\":\"COLOURS\"}", "Favourite COLOURS"},
		{"text.yaml", "# Example: favourite colour\ncolour: Customise your favourite colour # grey\n", "Customise favourite colour"},
		{"guide.md", "Choose a colour. Use `colour`.\n~~~go\ncolour\n~~~\nFavourites\n", "colour Favourites"},
		{"manual.unknown", "Favourite colour, favourite_colour, colour2, myFavourite. Reorganised!", "Favourite colour Reorganised"},
		{"safe.txt", "tour acre exercise surprise analysis promise cancellation license practice favouriteColour", ""},
	} {
		t.Run(tc.file, func(t *testing.T) {
			got := []string{}
			scan(tc.file, tc.src, "auto", dictionary(), nil, func(f finding) { got = append(got, f.Word) })
			if strings.Join(got, " ") != tc.want {
				t.Errorf("got %v, want %s", got, tc.want)
			}
		})
	}
}
func TestPositionsAndRanges(t *testing.T) {
	src := "colour\r\n🐈 É  FAVOURITES\r\ncolour\n🧪  Centre colour"
	var got []finding
	scan("x.txt", src, "text", dictionary(), []lineRange{{2, 2}, {4, 4}}, func(f finding) { got = append(got, f) })
	want := []finding{
		{"x.txt", 2, 6, "FAVOURITES", "FAVORITES", "🐈 É  FAVOURITES", nil, 16},
		{"x.txt", 4, 4, "Centre", "Center", "🧪  Centre colour", nil, 10},
		{"x.txt", 4, 11, "colour", "color", "🧪  Centre colour", nil, 17},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}
func TestCLIMarkedSource(t *testing.T) {
	src := "// colour\r\n\tshow(\"Favourite colour\"); // colour  \r\nshow(\"🐈 e\u0301 colour colour\");"
	var out, diag bytes.Buffer
	status := run([]string{"--stdin-name", "ui.js", "-"}, strings.NewReader(src), &out, &diag)
	want := "ui.js:2:8: Favourite → Favorite\nui.js:2:18: colour → color\n" +
		"  | \tshow(\"Favourite colour\"); // colour  \n  | \t      ^^^^^^^^^ ^^^^^^\n" +
		"ui.js:3:12: colour → color\nui.js:3:19: colour → color\n" +
		"  | show(\"🐈 e\u0301 colour colour\");\n  | show(\"🐈 e\u0301 [[colour]] [[colour]]\");\n"
	if status != 1 || out.String() != want || diag.Len() != 0 {
		t.Fatalf("status %d, stdout %q, stderr %q", status, out.String(), diag.String())
	}
}
func TestHelp(t *testing.T) {
	for _, option := range []string{"--help", "-h"} {
		var out, diag bytes.Buffer
		status := run([]string{option, "--terms", "absent/terms.txt", "-"}, nil, &out, &diag)
		if status != 0 || diag.Len() != 0 {
			t.Fatalf("%s: status %d, stderr %q", option, status, diag.String())
		}
		for _, topic := range []string{"Usage: uscopylint", "--changed HEAD", "--output JSON", "sourceLine", "endColumn", "Exit 1"} {
			if !strings.Contains(out.String(), topic) {
				t.Errorf("%s: help is missing %q", option, topic)
			}
		}
	}
}

func TestVersionSkipsScan(t *testing.T) {
	var out, diag bytes.Buffer
	status := run([]string{"--version", "--terms", "absent/terms.txt", "-"}, nil, &out, &diag)
	fields := strings.Fields(out.String())
	if status != 0 || diag.Len() != 0 || len(fields) != 2 || fields[0] != "uscopylint" || out.String() != strings.Join(fields, " ")+"\n" {
		t.Fatalf("status %d, stdout %q, stderr %q", status, out.String(), diag.String())
	}
}

func TestCLI(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		src      string
		status   int
		contains string
	}{
		{[]string{"--format", "json", "--stdin-name", "ui.jsx", "-"}, "const colour = <p>Colour</p>", 1, `"suggestion": "Color"`},
		{[]string{"--output", "JSON", "-"}, "Cheque memo", 1, `"suggestion": "Check memo"`},
		{[]string{"--output=JsOn", "-"}, "Check memo", 0, "[]"},
		{[]string{"--output", "TEXT", "-"}, "colour", 1, "stdin.txt:1:1: colour → color"},
		{[]string{"--output", "yaml", "-"}, "colour", 2, "invalid output format"},
		{[]string{"--ignore", "COLOUR", "-"}, "colour", 0, ""},
		{[]string{"--mode", "text", "--stdin-name", "a.js", "-"}, "// colour", 1, "a.js:1:4: colour → color"},
		{[]string{"-"}, "\x00colour", 0, ""},
		{[]string{"-"}, "\xff\xfec\x00o\x00l\x00o\x00u\x00r\x00", 1, "stdin.txt:1:1"},
		{[]string{"--format", "json", "-"}, "color", 0, "[]"},
		{[]string{"--changed", "HEAD", "--staged"}, "", 2, "invalid"},
		{[]string{"--format", "yaml"}, "", 2, "invalid"},
		{[]string{"absent/path.txt"}, "", 2, "uscopylint:"},
	} {
		var output bytes.Buffer
		status := run(tc.args, strings.NewReader(tc.src), &output, &output)
		if status != tc.status || !strings.Contains(output.String(), tc.contains) {
			t.Errorf("%v: %d %q", tc.args, status, output.String())
		}
	}
}
func TestDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "vendor"), 0755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{"sample.md": "colour", "vendor/deps.txt": "colour", "binary.bin": "\x00colour"} {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	status := run([]string{"--format", "json", dir, filepath.Join(dir, "sample.md")}, strings.NewReader(""), &out, &out)
	var got []finding
	if status != 1 || json.Unmarshal(out.Bytes(), &got) != nil || len(got) != 1 {
		t.Fatalf("%d %s", status, out.String())
	}
}
