# uscopylint

uscopylint is a Go CLI that checks user-facing text for British spellings and regional phrases. Point it at an application checkout to find wording such as *cheque memo* in a payment form or *shopping trolley* split across JSX tags. It reads inline copy directly, so it also works in applications without translation files or centralized string constants.

Use it in CI to check just the lines a pull request changes. Human-readable output marks the offending words on their source lines. JSON gives an editor the exact source spans and replacement text. The scanner runs locally and uses the Go standard library.

## Quick start

From this source directory, run the following with Go 1.22 or later. The install command places the executable in Go’s binary directory. The examples assume that directory is on your PATH.

```sh
go test ./...
go install .
```

Try it with an inline JSX label. The command reports `trolley → cart` on line 1 and exits with status 1 because it found a match.

```sh
printf '%s\n' '<label>Shopping <b>trolley</b></label>' | uscopylint --stdin-name Cart.tsx -
```

To check your current edits, run this from your application repository.

```sh
uscopylint --changed HEAD
```

CI logs show the file, line, column, and US spelling for each match. Each failing source line is printed once, with every detected word marked.

```text
src/Settings.tsx:8:9: Favourite → Favorite
src/Settings.tsx:8:19: colour → color
  | <button>Favourite colour</button>
  |         ^^^^^^^^^ ^^^^^^
```

## Workflows

| Command | What it checks |
| --- | --- |
| `uscopylint --changed HEAD` | Added and modified working-tree lines relative to HEAD, plus nonignored untracked files. |
| `uscopylint --staged` | Added and modified lines in the staged snapshot, including the first commit. |
| `uscopylint --changed "$base" src` | Changes under a specific path since a chosen commit. |
| `uscopylint src pages/help.md` | Complete files or directory trees. With no paths, scans the current directory. |
| `uscopylint --mode text path/to/file` | All words, including comments and identifiers that are standalone words. |
| `uscopylint --mode code schema.xyz` | Quoted strings in a file with an unfamiliar extension. |
| `uscopylint --stdin-name Settings.tsx -` | Standard input, classified and reported under the supplied name. |
| `uscopylint --output "JSON" --changed "$base"` | A JSON array on stdout with one entry per source edit, including the source line, exact span, and replacement. |

Pass flags before paths. Shell-expanded file lists and literal paths are supported. Git modes default to the current directory and its descendants, and report paths relative to the repository root. They use Git’s own added-line ranges, so an edited line can also report an older spelling elsewhere on that same line. The scanner reads the surrounding file to interpret multiline strings and templates.

## CI integration

For a pull-request check, fetch the target branch and enough history to resolve its merge base, then run the check in the checkout with the executable on PATH.

```sh
base=$(git merge-base origin/main HEAD) || exit 2
uscopylint --changed "$base"
```

Exit code **0** means clean, **1** means findings, and **2** means a usage, input, or Git error. A clean JSON run prints `[]`. In each JSON finding, `sourceLine` contains the decoded source line with its indentation and trailing spaces. The line terminator is omitted. `line` and `column` start at 1. The word spans `column` through `endColumn - 1`, counted in Unicode code points.

Text diagnostics use plain carets and preserve tabs for alignment. For lines containing non-ASCII characters, a second copy uses `[[word]]` markers so the match is clear regardless of how the CI viewer renders character widths.

## JSON for automated fixes

For a CI correction step, use `--output "JSON"` and pass stdout to the editor on exit **1**. Exit **0** means the scan is clean. On exit **2**, read stderr for the error. Format names are case-insensitive and `--format` is an alias for `--output`.

Each JSON entry describes one edit to the scanned snapshot. For example, scanning `<label>Shopping <b>trolley</b></label>` produces this finding.

```json
{
  "path": "Cart.tsx",
  "line": 1,
  "column": 20,
  "endColumn": 27,
  "word": "trolley",
  "suggestion": "cart",
  "sourceLine": "<label>Shopping <b>trolley</b></label>",
  "phrase": {
    "word": "Shopping trolley",
    "suggestion": "Shopping cart",
    "line": 1,
    "column": 8
  }
}
```

Use the top-level `column`, `endColumn`, and `suggestion` for edits. The optional `phrase` describes the combined JSX text and points to its first source character. Several edits can share the same `path`, `phrase.line`, and `phrase.column`. Apply those edits together, including any whose suggestion is an empty string. In Git modes, a changed line anywhere from the first to the last word of a split phrase can trigger the whole correction.

Before editing a file, verify every reported `sourceLine` against the same snapshot. Replace the indicated spans from the highest column down on each line, then rerun the linter. The example above becomes `<label>Shopping <b>cart</b></label>`.

## How copy is selected

- In common source formats, the scanner looks inside quoted strings, including inline messages and single-word button labels. It recognizes common line and block comments, JavaScript template literals, and several multiline string forms.
- In HTML and JSX it reads element text and accessibility or display attributes such as `alt`, `aria-label`, `title`, and `placeholder`. For example, `<button className="colour" title="Favourite">Customise</button>` reports **Favourite** and **Customise**. It also scans quoted copy inside common template expressions.
- In JSX, phrases can cross fragments and inline formatting elements such as `span`, `strong`, `em`, and `b`. Unescaped quoted child expressions, including `{' '}`, supply literal text. JSX block comments contribute no text. Adjacent tags contribute no implied space. Components, controls, block elements, and dynamic expressions separate text runs.
- Code scans skip the arguments of `console.debug/log/trace/info/warn/error` and debug or trace methods on `logger`, `log`, `logging`, `Logger`, `LOGGER`, and `slog`. This includes forms such as `logger.Debugf` and `slog.DebugContext`. The next call on the same line is checked normally. Calls on unfamiliar receivers and standalone functions such as `debug()` are scanned. Use `--mode text` to include log messages.
- JSON values, common YAML values, and Markdown prose have tailored handling. Unrecognized extensions are scanned as plain text. Use `--mode markup`, `--mode code`, or `--mode text` to override classification.
- Regular scans read UTF-8 (with or without a BOM) and BOM-marked UTF-16. Binary data and symlinks are skipped. Recursive filesystem scans skip common VCS and dependency directories such as `.git`, `node_modules`, and `vendor`. Git modes respect Git’s ignore rules for untracked files and use text hunks for tracked changes. For a file Git treats as binary, use a whole-file scan.

Visibility is inferred from source syntax. Internal string values and proper names can produce findings. Dynamically assembled words, escaped spellings, and unfamiliar template syntax can escape detection. The dictionary covers selected UK spellings and US style preferences, including variants such as *grey* and *towards* that also occur in US English. Foreign-language identification and context-dependent vocabulary require a separate review.

## Vocabulary coverage

The built-in vocabulary contains **890 distinct single-word forms** (including inflections) and **86 multiword rules**. The tests run every rule through the scanner and check that its suggested replacement passes. Coverage of a particular application also depends on its terminology and how it renders copy.

The 140 additions from the Codespell 2.4.3 review cover wording such as *greyscale*, *organisational*, *routeing* and *journalling*. The review covered 186 candidate forms. The other 46 are house-style choices, context-dependent terms or low-priority specialist wording. For example, *dialogue* is allowed in a transcript, while *dialogue box* still suggests *dialog box*.

The reviewed subset in `spellings-codespell.txt` adapts dictionary data by Codespell contributors derived from English Wikipedia. It is distributed under CC BY-SA 3.0. [Codespell 2.4.3](https://pypi.org/project/codespell/2.4.3/) · [Dictionary license](https://creativecommons.org/licenses/by-sa/3.0/)

| Source wording | US suggestion |
| --- | --- |
| Cheque memo | Check memo |
| Paying-in slip | Deposit slip |
| Card expiry date | Card expiration date |
| Shopping trolley | Shopping cart |
| Tick the box | Check the box |

At each word, the longest matching rule wins. For example, `Cheque memo line` produces one finding spanning all three words. Spaces, tabs, and nonbreaking spaces can separate words. Straight and curly apostrophes match the same rule, as do standard and nonbreaking hyphens. Ordinary copy spans end at a source line break, a separate string, an interpolation, or an HTML tag. JSX can join static children across source lines when their text contains a separator, including an explicit JSX space. A line break inside a single JSX text node still separates the checks.

## Tune the vocabulary

Keep product-specific terms in a UTF-8 file and pass it to CI. Each entry can introduce a word or phrase or override an existing suggestion. For example, if the product calls its cheque memo field a *payment note*, use the following file.

```text
# billing-terms.txt
cheque memo | payment note
remittance advice | payment notice
```

```sh
uscopylint --terms billing-terms.txt --changed "$BASE_SHA"
```

Use one `UK wording | US wording` entry per line. Blank lines and lines beginning with `#` are skipped. The last entry for a term wins. The supplied glossary is excluded from input scans. Invalid entries report the configuration line and exit with code 2.

Use `--ignore "grey,cheque memo"` to allow individual words or phrases. A single allowed word also disables phrase rules containing that word. Matching ignores case and preserves capitalization in suggestions. An ignore applies to the exact inflected form supplied.

The embedded single-word rules live in `spellings.txt` and `spellings-codespell.txt`. The built-in phrase rules live in `phrases.txt`. To add your own embedded spellings, edit `spellings.txt` and rebuild. Each row gives the British form, the US form, and a comma-separated list of endings. A dash includes the bare form. For example, `colour color -,s,ed,ing` checks four spellings including *coloured* → *colored*. Rules apply to whole words, so *colour* inside `colourPicker` or `button_colour` is left alone.
