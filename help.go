package main

const helpText = `uscopylint checks user-facing copy for British spellings and suggests US wording.
It reads source files locally and reports proposed edits. You apply the fixes.

Usage: uscopylint [options] [file|directory ...]
Put options before paths. With no paths, scan the current directory recursively.
Use - as a path to read stdin. Give the input a filename to select its syntax.

Examples
  uscopylint src docs/help.md
  uscopylint --changed HEAD --output JSON
  uscopylint --staged
  printf '%s\n' '<label>Shopping <b>trolley</b></label>' | uscopylint --stdin-name Cart.tsx --output JSON -

The last command reports one edit in Cart.tsx on line 1, columns [20, 27).
Changing "trolley" to "cart" keeps the JSX tags intact.

Choosing what to scan
  --changed REV      Check added or modified working-tree lines against a Git
                     commit, plus nonignored untracked files. For a PR, use the
                     merge base of the target branch and the checked-out PR head.
  --staged           Check added or modified lines in the staged Git snapshot.
                     Choose one Git mode per run. Both accept paths to narrow the
                     scan, report paths from the repository root, and require Git.
                     Use stdin in a separate scan.
  --stdin-name NAME  Classify and label stdin using NAME (default stdin.txt).
  --mode MODE        auto (default) uses the filename to select likely copy,
                     including UI strings and visible JSX. Unknown types use text.
                     code scans quoted strings and skips recognized debug calls.
                     markup scans element text and display attributes.
                     text checks all words, including those in comments and logs.

Options
  --output FORMAT    text (default) marks matches on source lines. JSON writes an
                     array of proposed edits to stdout. Format is case-insensitive.
  --format FORMAT    Alias for --output.
  --terms FILE       Load custom rules, one "UK wording | US wording" per line.
  --ignore LIST      Allow comma-separated words or phrases across all files.
  --allow-in RULE    Allow wording in one exact file. Repeat as needed, e.g.
                     --allow-in 'src/Address.tsx=Grey Street'.
  --version          Print the build version and exit.
  -h, --help         Print this guide and exit.

Using JSON findings
  Exit 0: clean scan. JSON output is [].
  Exit 1: findings. Read the JSON array from stdout even though the exit is nonzero.
  Exit 2: command or scan error. Read stderr for the diagnostic.

  Each finding identifies a path and a 1-based line. Before editing, verify that
  the line still matches sourceLine in the scanned snapshot. Columns count Unicode
  code points starting at 1. Replace word in [column, endColumn) with suggestion.
  An empty suggestion means delete that span. For JSX, phrase gives context:
  apply all associated top-level edits together. Work right to left on each line,
  then rerun the scan. For --staged, stage the intended fixes before rerunning.
`
