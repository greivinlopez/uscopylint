package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestGitModes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Error(err)
		}
	})
	g := func(args ...string) {
		t.Helper()
		if _, err := git("", args...); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want []string, args ...string) {
		t.Helper()
		var out, diag bytes.Buffer
		status := run(append([]string{"--format", "json"}, args...), strings.NewReader(""), &out, &diag)
		var findings []finding
		if err := json.Unmarshal(out.Bytes(), &findings); err != nil {
			t.Fatalf("%v: %v: %s", args, err, diag.String())
		}
		var got []string
		for _, f := range findings {
			got = append(got, f.Path+" "+f.Word)
			line := []rune(f.SourceLine)
			if f.Column < 1 || f.EndColumn > len(line)+1 || f.Column >= f.EndColumn ||
				string(line[f.Column-1:f.EndColumn-1]) != f.Word {
				t.Fatalf("%v: invalid JSON source span %+v", args, f)
			}
			if f.Word == "Favourite" && (f.Line != 2 || f.SourceLine != "My Favourite") {
				t.Fatalf("%v: expected staged source line, got %+v", args, f)
			}
		}
		if !reflect.DeepEqual(got, want) || (len(want) == 0 && status != 0) || (len(want) > 0 && status != 1) {
			t.Fatalf("%v: got %v (%d), want %v", args, got, status, want)
		}
	}
	g("init", "-q")
	g("config", "user.name", "Test")
	g("config", "user.email", "test@example.test")
	g("config", "commit.gpgsign", "false")
	g("config", "core.autocrlf", "false")
	write(".gitignore", "ignored.txt\n")
	if err := os.Mkdir("app", 0755); err != nil {
		t.Fatal(err)
	}
	base := "const msg = `Old colour\nplain\nlast`;\n"
	write("app/view.js", base)
	write("old.txt", "Old favourite\n")
	write("delete.txt", "colour")
	g("add", ".")
	check([]string{"app/view.js colour", "delete.txt colour", "old.txt favourite"}, "--staged")
	g("commit", "-qm", "base")
	g("mv", "old.txt", "renamed.txt")
	g("rm", "-q", "delete.txt")
	write("app/view.js", strings.Replace(base, "plain", "My Favourite", 1))
	g("add", "app/view.js")
	write("app/view.js", strings.Replace(base, "plain", "Customise", 1))
	name := "app/new file.txt"
	if runtime.GOOS != "windows" {
		name = "app/new\tfile\n.txt"
	}
	write(name, "Unrecognised")
	write("ignored.txt", "colour")
	check([]string{"app/view.js Favourite"}, "--staged")
	check([]string{name + " Unrecognised", "app/view.js Customise"}, "--changed", "HEAD")
	if err := os.Chdir("app"); err != nil {
		t.Fatal(err)
	}
	check([]string{"app/view.js Customise"}, "--changed", "HEAD", "view.js")
	if err := os.Chdir(".."); err != nil {
		t.Fatal(err)
	}
	g("add", name)
	check([]string{name + " Unrecognised", "app/view.js Favourite"}, "--staged")
	write("app/terms.txt", "payment advice | payment notice\n")
	write("app/view.js", strings.Replace(base, "plain", "Cheque memo", 1))
	g("add", "app/view.js")
	write("app/view.js", strings.Replace(base, "plain", "Payment advice", 1))
	check([]string{"app/view.js Cheque memo"}, "--staged", "--terms", "app/terms.txt", "app/view.js")
	check([]string{name + " Unrecognised", "app/view.js Payment advice"}, "--changed", "HEAD", "--terms", "app/terms.txt", "app")
	// A staged change to the first word can introduce a phrase whose only
	// editable word is on the next (unchanged) line.
	template := "<label><span>Garden </span>\n<strong>trolley</strong></label>\n"
	write("ui.tsx", template)
	g("add", "ui.tsx")
	g("commit", "-qm", "JSX fixture", "--", "ui.tsx")
	write("ui.tsx", strings.Replace(template, "Garden", "Shopping", 1))
	check([]string{"ui.tsx trolley"}, "--changed", "HEAD", "ui.tsx")
	g("add", "ui.tsx")
	write("ui.tsx", template)
	check([]string{"ui.tsx trolley"}, "--staged", "ui.tsx")
	check(nil, "--changed", "HEAD", "ui.tsx")
}
