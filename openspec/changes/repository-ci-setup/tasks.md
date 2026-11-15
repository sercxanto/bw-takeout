# Tasks

## 1. License

- [x] 1.1 Add `LICENSE` with the MIT license text and `Copyright (c) 2026 Georg Lutz` (design D10); verify the text matches the MIT license as published by the Open Source Initiative, apart from year and holder
- [x] 1.2 Replace the license sentence in `README.md` with a reference to `LICENSE`; verify the README names MIT and links the file
- [x] 1.3 Update `docs/development/releases.md`: license settled, the three-clause BSD licenses of age, `golang.org/x/crypto` and `golang.org/x/sys`, and that their notices must ship with a binary release; verify each named license against the `LICENSE` file in the module cache

## 2. Skip report

- [x] 2.1 Implement `internal/tools/skipreport` with the standard library only (design D7): read `go test -json` from standard input, echo the test output, collect skipped tests with their output, write a summary table to `GITHUB_STEP_SUMMARY` when set and to standard output otherwise; verify a test feeds a recorded stream and compares the echoed output and the table
- [x] 2.2 Read `.github/expected-skips.txt` with platform, package, test and reason, and exit with status 1 on a failed test and on a skip the list does not allow for `GOOS`; verify one test each for an expected skip, an expected skip for `*`, an unexpected skip, a skip expected only on another platform, a failed test, an entry that did not skip, a line without reason and a malformed line
- [x] 2.3 Create `.github/expected-skips.txt` with the entries of design D7 and a reason for each; verify `go test -count=1 -json ./... | go run ./internal/tools/skipreport` exits with status 0 in the dev container
- [x] 2.4 Verify the failure case end to end: run the same pipe with a search path that lacks `age`, so that `TestRecoveryWithStandardTools` skips, and check that the report names it with its reason and exits with status 1
- [x] 2.5 Verify with `go list -deps ./cmd/bw-takeout` that the command does not depend on `internal/tools/skipreport`, and that the dependency check of the command still passes
- [x] 2.6 Describe the skip report, the list of expected skips and the rule for adding an entry in `docs/development/testing.md`; verify the documented command runs as written

## 3. Workflow

- [x] 3.1 Resolve the commit SHAs of the newest `actions/checkout` and `actions/setup-go` release tags with `git ls-remote`, following annotated tags to their commit; verify each SHA has 40 hexadecimal digits and record tag and SHA for 3.2
- [x] 3.2 Write `.github/workflows/ci.yml` with the jobs, triggers, runners, pins and permissions of design D1 to D6 and D8; verify by reading that every `uses:` names a full SHA with the tag as comment, every `runs-on` a versioned label, that `permissions` is read-only, that `persist-credentials` is false, that `cache` is false, and that the file names no secret
- [x] 3.3 Run each `run:` step of both jobs in the dev container in `bash -eo pipefail` exactly as written; verify every step succeeds, and that the format check, `go mod tidy -diff` and the key-material search fail when given a misformatted file, an untidy `go.mod` and an untracked file with a key prefix, each undone afterwards
- [x] 3.4 Parse `ci.yml` with a YAML parser available in the dev container, or with a minimal Go test if none is; verify the file parses and the matrix holds exactly the three runners
- [x] 3.5 Record in `docs/development/integrations.md` the pinned actions with tag and SHA, the runner labels and how to update them together with the other pins; verify the page names each pin of `ci.yml`
- [x] 3.6 Add to `docs/development/getting-started.md` where CI results and the skip report are found, and to `CONTRIBUTING.md` that a pull request names its CI run; verify both pages link the test strategy

## 4. First CI run

- [ ] 4.1 Maintainer step: create a private GitHub repository, add it as remote and push the branch with this change; verify `git remote -v` shows it and a run of the workflow appears
- [ ] 4.2 Review the first run per job: test results, race results, the skip report and the age version per runner; verify the result per platform is stated with the failures found, if any
- [ ] 4.3 Handle failures by the rules of design D9, each test fix or product fix with a regression test and listed in this task; verify a later run passes on all three runners with only expected skips
- [ ] 4.4 Record the passing run in `docs/development/testing.md`: date, runner labels and image versions from the log, Go and age versions, skips per platform, duration per job and the minutes used; update the list of what was not run; verify each statement is taken from the run log

## 5. Integration check

- [ ] 5.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...` and `go test -race ./...` in the dev container after all fixes; verify all pass and gofmt prints nothing
- [ ] 5.2 Run `openspec validate repository-ci-setup --strict`; verify it passes

## Workflow follow-up

- The maintainer reviews `ci.yml` and the expected skips before the first push.
- Archive the change after review and after a passing run on all three runners.
- Verify the archived result.
- Manual steps that CI does not cover stay open: the offline exercise on a second system and a run on FAT or exFAT media.
