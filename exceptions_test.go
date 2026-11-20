package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileExceptions(t *testing.T) {
	dir := t.TempDir()
	address := filepath.Join(dir, "address=home.tsx")
	other := filepath.Join(dir, "picker.tsx")
	src := "<p>Grey <b>Street</b>. Grey. Cheque memo. Colour.</p>"
	for _, p := range []string{address, other} {
		if err := os.WriteFile(p, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"--output", "JSON", "--allow-in", address + "=grey street", "--allow-in", address + "=cheque memo", "--ignore", "colour"}
	for _, tc := range []struct {
		paths []string
		want  string
	}{
		{[]string{address, other}, "Grey|Grey|Grey|Cheque memo"},
		{[]string{"--stdin-name", address, "-"}, "Grey"},
		{[]string{"--stdin-name", other, "-"}, "Grey|Grey|Cheque memo"},
	} {
		var out, diag bytes.Buffer
		status := run(append(args, tc.paths...), strings.NewReader(src), &out, &diag)
		var got []finding
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || status != 1 {
			t.Fatalf("exit %d: %v %s", status, err, diag.String())
		}
		var words []string
		for _, f := range got {
			words = append(words, f.Word)
		}
		if strings.Join(words, "|") != tc.want {
			t.Fatalf("%v: %+v", tc.paths, got)
		}
	}
}

func TestInvalidFileException(t *testing.T) {
	for _, value := range []string{"missing-delimiter", "=grey", "f=", "f=grey,street"} {
		var out, diag bytes.Buffer
		if code := run([]string{"--allow-in", value, "-"}, strings.NewReader(""), &out, &diag); code != 2 || out.Len() != 0 || !strings.Contains(diag.String(), "allow-in") {
			t.Fatalf("%q: exit %d, %s", value, code, diag.String())
		}
	}
}
