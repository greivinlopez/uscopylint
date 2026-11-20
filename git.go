package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"--literal-pathspecs"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return b, nil
}
func pathsZ(out []byte) []string {
	if len(out) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

var hunk = regexp.MustCompile(`(?m)^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

func addedRanges(patch []byte) []lineRange {
	ranges := []lineRange{}
	for _, m := range hunk.FindAllSubmatch(patch, -1) {
		first, _ := strconv.Atoi(string(m[1]))
		count := 1
		if len(m[2]) > 0 {
			count, _ = strconv.Atoi(string(m[2]))
		}
		if count > 0 {
			ranges = append(ranges, lineRange{first, first + count - 1})
		}
	}
	return ranges
}

func gitFiles(paths []string, o options, check func(string, string, []byte, []lineRange)) error {
	top, err := git("", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	root := strings.TrimSuffix(string(top), "\n")
	// Resolve caller paths relative to the root once. Git output and index
	// lookups are subsequently all root-relative, including from subfolders.
	if len(paths) == 0 {
		paths = []string{"."}
	}
	specs := []string{}
	for _, p := range paths {
		if p == "-" {
			return fmt.Errorf("stdin cannot be combined with Git modes")
		}
		abs, e := filepath.Abs(p)
		if e != nil {
			return e
		}
		p, e = filepath.Rel(root, abs)
		if e != nil {
			return e
		}
		if p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
			return fmt.Errorf("path outside Git worktree: %s", abs)
		}
		specs = append(specs, filepath.ToSlash(p))
	}
	unmerged, err := git(root, append([]string{"ls-files", "--unmerged", "-z", "--"}, specs...)...)
	if err != nil {
		return err
	}
	if len(unmerged) > 0 {
		return fmt.Errorf("resolve merge conflicts in the selected paths before checking changes")
	}
	diff := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--ignore-submodules=all"}
	if o.staged {
		diff = append(diff, "--cached")
	} else {
		b, e := git(root, "rev-parse", "--verify", "--end-of-options", o.changed+"^{commit}")
		if e != nil {
			return e
		}
		diff = append(diff, strings.TrimSpace(string(b)))
	}
	// Force stable patch headers even when the repository customizes prefixes.
	args := append(diff, "--patch", "--no-relative", "--src-prefix=a/", "--dst-prefix=b/", "--line-prefix=", "--unified=0", "--inter-hunk-context=0", "--output-indicator-new=+", "--output-indicator-old=-", "--output-indicator-context= ", "--")
	patch, err := git(root, append(args, specs...)...)
	if err != nil {
		return err
	}
	files, err := parsePatch(patch)
	if err != nil {
		return err
	}
	if !o.staged {
		b, e := git(root, append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, specs...)...)
		if e != nil {
			return e
		}
		for _, p := range pathsZ(b) {
			files[p] = nil
		}
	}
	names := make([]string, 0, len(files))
	for p := range files {
		if filepath.Join(root, filepath.FromSlash(p)) != o.terms {
			names = append(names, p)
		}
	}
	sort.Strings(names)
	for _, p := range names {
		absolute := filepath.Join(root, filepath.FromSlash(p))
		var b []byte
		if o.staged {
			b, err = git(root, "cat-file", "blob", ":0:"+p)
		} else {
			info, e := os.Lstat(absolute)
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				continue
			}
			b, err = readFile(absolute)
		}
		if err != nil {
			return err
		}
		check(p, absolute, b, files[p])
	}
	return nil
}

// The +++ header contains the destination path. Git quotes tabs, newlines
// and other special bytes with C-style escapes. Only inspect patch headers,
// since an added source line can itself begin with "++ ".
func parsePatch(patch []byte) (map[string][]lineRange, error) {
	out := map[string][]lineRange{}
	for _, section := range bytes.Split(patch, []byte("\ndiff --git ")) {
		header, _, hasHunks := bytes.Cut(section, []byte("\n@@ "))
		if !hasHunks {
			continue
		}
		var path string
		symlink := false
		for _, line := range strings.Split(string(header), "\n") {
			switch {
			case strings.HasPrefix(line, "+++ "):
				path = strings.TrimSuffix(line[4:], "\t")
				if strings.HasPrefix(path, "\"") {
					var err error
					path, err = strconv.Unquote(path)
					if err != nil {
						return nil, fmt.Errorf("invalid Git path %q: %w", path, err)
					}
				}
			case line == "new file mode 120000", line == "new mode 120000",
				strings.HasPrefix(line, "index ") && strings.HasSuffix(line, " 120000"):
				symlink = true
			}
		}
		if path == "/dev/null" || symlink {
			continue
		}
		if !strings.HasPrefix(path, "b/") {
			return nil, fmt.Errorf("missing Git destination path in patch")
		}
		ranges := addedRanges(section)
		if len(ranges) > 0 {
			out[path[2:]] = ranges
		}
	}
	return out, nil
}
