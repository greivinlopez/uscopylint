package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type options struct {
	mode, format, name, changed, ignore, terms string
	staged                                     bool
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
func run(args []string, in io.Reader, out, errOut io.Writer) int {
	var o options
	f := flag.NewFlagSet("uscopylint", flag.ContinueOnError)
	f.SetOutput(errOut)
	showVersion := f.Bool("version", false, "print the build version and exit")
	f.StringVar(&o.mode, "mode", "auto", "auto, code, markup, or text (check every word)")
	f.StringVar(&o.format, "output", "text", "output format: text or JSON (case-insensitive)")
	f.StringVar(&o.format, "format", "text", "alias for --output")
	f.StringVar(&o.name, "stdin-name", "stdin.txt", "filename used to classify and report stdin")
	f.StringVar(&o.changed, "changed", "", "check added/modified working-tree lines since a Git revision, plus untracked files")
	f.BoolVar(&o.staged, "staged", false, "check added/modified lines in the Git index")
	f.StringVar(&o.ignore, "ignore", "", "comma-separated words or phrases to allow (case-insensitive)")
	fileExceptions := make(map[string][]string)
	f.Func("allow-in", "allow a word or phrase in one exact file: PATH=WORDING (repeatable)", func(value string) error {
		delimiter := strings.LastIndexByte(value, '=')
		if delimiter <= 0 {
			return fmt.Errorf("expected PATH=WORDING")
		}
		path := value[:delimiter]
		wording := strings.TrimSpace(value[delimiter+1:])
		if err := validateTerm(wording); err != nil {
			return err
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		fileExceptions[absolute] = append(fileExceptions[absolute], wording)
		return nil
	})
	f.StringVar(&o.terms, "terms", "", "additional UK | US terminology file")
	f.Usage = func() {
		fmt.Fprintln(errOut, "Usage: uscopylint [options] [file|directory ...]\nUse - for stdin. With no paths, scan the current directory.\nExit: 0 clean, 1 findings, 2 error.")
		f.PrintDefaults()
	}
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *showVersion {
		version := "(devel)"
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
		}
		if _, err := fmt.Fprintln(out, "uscopylint", version); err != nil {
			fmt.Fprintln(errOut, "uscopylint:", err)
			return 2
		}
		return 0
	}
	o.format = strings.ToLower(o.format)
	if o.format != "text" && o.format != "json" {
		fmt.Fprintf(errOut, "uscopylint: invalid output format %q (use text or JSON)\n", o.format)
		return 2
	}
	if !oneOf(o.mode, "auto text code markup") || o.changed != "" && o.staged {
		fmt.Fprintln(errOut, "uscopylint: invalid mode or combination of --changed and --staged")
		return 2
	}
	words := dictionary()
	if o.terms != "" {
		path, err := filepath.Abs(o.terms)
		if err == nil {
			o.terms = path
			var data []byte
			data, err = os.ReadFile(path)
			if err == nil {
				err = words.loadTerms(string(data))
			}
		}
		if err != nil {
			fmt.Fprintf(errOut, "uscopylint: %s: %v\n", o.terms, err)
			return 2
		}
	}
	for _, word := range strings.Split(o.ignore, ",") {
		words.allow(strings.TrimSpace(word))
	}
	found := []finding{}
	check := func(path, absolute string, data []byte, ranges []lineRange) {
		s, ok := decodeText(data)
		if !ok {
			return
		}
		scan(path, s, o.mode, words.withExceptions(fileExceptions[absolute]), ranges, func(f finding) { found = append(found, f) })
	}
	paths := f.Args()
	var err error
	if o.staged || o.changed != "" {
		err = gitFiles(paths, o, check)
	} else {
		if len(paths) == 0 {
			paths = []string{"."}
		}
		seen := map[string]bool{}
		for _, path := range paths {
			if path == "-" {
				if seen["\x00"] {
					continue
				}
				seen["\x00"] = true
				var b []byte
				b, err = io.ReadAll(in)
				if err == nil {
					var absolute string
					if len(fileExceptions) > 0 {
						absolute, err = filepath.Abs(o.name)
					}
					if err == nil {
						check(o.name, absolute, b, nil)
					}
				}
			} else {
				err = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if d.IsDir() {
						if p != path && oneOf(d.Name(), ".git .hg .svn node_modules vendor .venv __pycache__") {
							return filepath.SkipDir
						}
						return nil
					}
					if !d.Type().IsRegular() {
						return nil
					}
					abs, e := filepath.Abs(p)
					if e != nil {
						return e
					}
					if seen[abs] || abs == o.terms {
						return nil
					}
					seen[abs] = true
					b, e := readFile(p)
					if e == nil {
						check(p, abs, b, nil)
					}
					return e
				})
			}
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		fmt.Fprintln(errOut, "uscopylint:", err)
		return 2
	}
	if o.format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		err = enc.Encode(found)
	} else {
		err = writeDiagnostics(out, found)
	}
	if err != nil {
		fmt.Fprintln(errOut, "uscopylint:", err)
		return 2
	}
	if len(found) > 0 {
		return 1
	}
	return 0
}

// Sniff before loading a large binary into memory.
func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var head [8192]byte
	n, err := io.ReadFull(f, head[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	if !hasUTF16BOM(head[:n]) && bytes.IndexByte(head[:n], 0) >= 0 {
		return nil, nil
	}
	b := bytes.NewBuffer(append([]byte(nil), head[:n]...))
	_, err = b.ReadFrom(f)
	return b.Bytes(), err
}
func hasUTF16BOM(b []byte) bool {
	return bytes.HasPrefix(b, []byte{0xff, 0xfe}) || bytes.HasPrefix(b, []byte{0xfe, 0xff})
}
func decodeText(b []byte) (string, bool) {
	if hasUTF16BOM(b) {
		var order binary.ByteOrder = binary.LittleEndian
		if b[0] == 0xfe {
			order = binary.BigEndian
		}
		b = b[2:]
		if len(b)%2 != 0 {
			return "", false
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = order.Uint16(b[2*i:])
			if u[i] == 0 {
				return "", false
			}
		}
		return string(utf16.Decode(u)), true
	}
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}
