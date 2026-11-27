# Contributing to uscopylint

## Propose a wording rule

Open the [wording report form](https://github.com/greivinlopez/uscopylint/issues/new?template=wording-report.yml) and select **Missed wording (a finding was expected)**. Include the label or sentence a user sees, the US wording you expect, and a short explanation of why the replacement preserves the meaning. You can also send a pull request directly.

Choose the smallest rule that works in context. For example, `shopping trolley` can safely become `shopping cart` on a checkout screen. A transit app can legitimately say “trolley,” so the bundled rule matches the phrase. If the wording is specific to your product, try it in a [project glossary](README.md#tune-the-vocabulary) with `--terms`.

| Kind of change | Where it goes | Example from the dictionary |
| --- | --- | --- |
| Original single-word spelling | [spellings.txt](spellings.txt) | `behaviour behavior -,s` covers the singular and plural. The third column lists suffixes appended to both stems. A dash includes the bare form. |
| Phrase | [phrases.txt](phrases.txt) | `cheque memo \| check memo` matches the whole label. Add a separate row for a plural if it should match too. |
| Change to a Codespell-derived entry | [spellings-codespell.txt](spellings-codespell.txt) | Keep the source attribution and CC BY-SA 3.0 notice with the adapted data. |

For a pull request, show a before-and-after example. The tests exercise every bundled rule and check that its US replacement scans cleanly. If the change depends on context, include a regression case with wording that should be flagged and a nearby example that should pass. See [phrase tests](terms_test.go) and [JSX and debug-log tests](jsx_test.go).

## Report a false positive

Select **False positive (wording should be allowed)** in the [wording report form](https://github.com/greivinlopez/uscopylint/issues/new?template=wording-report.yml). Include the details below so a maintainer can reproduce the finding.

- Your `uscopylint --version` output, or the tag or commit if you use v0.1.0.
- The filename with its extension and the smallest snippet that reproduces the finding. Keep relevant quotes, JSX tags, or logging calls so someone else can reproduce the same scan.
- The exact command, any custom terms or ignores, and the actual finding. Explain which wording should stay and why.

To reproduce an issue from a snippet, pass the original filename with `--stdin-name`. This example reports only the edit from `trolley` to `cart` inside the bold tag.

```sh
printf '%s\n' '<label>Shopping <b>trolley</b></label>' | go run . --output JSON --stdin-name Cart.tsx -
```

## Run the tests locally

Install Go 1.22 or later and Git. From the root of your checkout, run the same commands as [project CI](.github/workflows/ci.yml).

```sh
go test ./...
go vet ./...
```

CI runs both commands on Ubuntu with Go 1.22.x and the current stable Go release. While editing the dictionary, you can run just its rule checks with `go test -run '^TestEveryBundledTerm$' ./...`.

Open your pull request against `main`. If you contribute from a fork, a maintainer approves the workflow run. Both Go checks must pass on an up-to-date branch before the PR can merge.

## Verify the install example for a release

Publishing a release starts the [release verification workflow](.github/workflows/verify-release.yml). To rerun it, open **Actions → Verify release → Run workflow** and enter the published tag, such as `v0.1.3`. An empty tag selects the latest release. Pull requests that edit the workflow also test the latest release.

The workflow installs the tag on Ubuntu, checks its version, and introduces a known wording change in a temporary Git repository. A passing run confirms that the CLI exited 1, reported exactly the expected `trolley` → `cart` edit, and uploaded `uscopylint.json` in the **uscopylint-release-findings** artifact. When updating the README pins, link the passing run for that version.

Dependabot checks the project's GitHub Actions weekly. When updating an action, also check the full commit SHA in the README example. Dependabot does not update Markdown code blocks.
