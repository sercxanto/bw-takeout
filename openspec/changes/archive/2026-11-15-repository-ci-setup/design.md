# Design

## Context

See [proposal.md](proposal.md) for motivation. The repository has no remote and no CI. Tests run with `go test ./...` in the dev container, which pins its Go image by digest and installs age v1.2.1 with `go install` ([integrations](../../../docs/development/integrations.md)).

Tests that depend on the platform, observed in the code on 15 November 2026:

- `internal/publish`: directory flush is skipped on Windows, and the rename is retried there. `TestPermissionsOnLinux` skips on Windows. `TestKillHelper` always reports a skip when it runs on its own, because it is the helper process of `TestProcessKilledWhileWriting`.
- `internal/archive`: `TestRecoveryWithStandardTools` skips when `age` or `tar` is missing from the search path.
- `cmd/bw-takeout`: `TestMain` builds the command with the Go toolchain from `runtime.GOROOT()`. `TestSystemWithoutBW` skips on Windows. `TestPartialGenerationByAnotherPath` skips where symbolic links cannot be created. `TestLeaveGenerationForOfflineExercise` skips without its environment variables.

Observed on 15 November 2026:

- The [runner images list](https://github.com/actions/runner-images) offers `ubuntu-24.04` (x64), `macos-26` (arm64) and `windows-2025` (x64) among others. `-latest` labels point to these at present.
- `git ls-remote` shows the tags `v7.0.1` of `actions/checkout` and `v7.0.0` of `actions/setup-go` as the newest.
- age, `golang.org/x/crypto` and `golang.org/x/sys` carry three-clause BSD licenses in the module cache.

## Goals / Non-Goals

**Goals:**

- The same tests as in the dev container, on three more platform combinations, on every push.
- A run that fails when a test fails or a test is skipped without a known reason.
- A workflow whose every input is pinned and that cannot write to the repository.

**Non-Goals:**

- Building or publishing release binaries, signing, or attaching artifacts to runs.
- Linux arm64 in CI. The dev container covers it.
- macOS on Intel and Windows on arm64.
- FAT, exFAT or network share targets, and the manual offline exercise.
- Automated updates of pins, such as Dependabot. Pins are updated by hand together with the other pins.
- Coverage reports and badges.

## Decisions

### D1: One workflow with two jobs

`.github/workflows/ci.yml` holds a job `test` with a matrix over the three runners and a job `checks` on Linux. `test` runs vet, the tests and the race tests. `checks` runs the platform-independent checks: formatting, module tidiness and the key-material search.

*Alternative considered:* all steps in the matrix job with conditions per platform. Rejected because a failed format check would then appear as a failed Linux test run.

### D2: Runners by versioned label

`ubuntu-24.04`, `macos-26` and `windows-2025`. A `-latest` label moves to a new image without a change in the repository, so a run could change result without a commit. A versioned label changes only with a commit. macOS arm64 matches the maintainer's host, and Windows amd64 matches the maintainer's Windows machine.

*Alternative considered:* `ubuntu-26.04`. Rejected for now because the label is new, and Ubuntu 24.04 is what `-latest` points to.

### D3: Pinned inputs

- Actions by full commit SHA with the tag as comment: `actions/checkout` and `actions/setup-go`. The SHA is resolved from the peeled tag with `git ls-remote` at implementation time. A tag can be moved, a commit SHA cannot.
- Go from `go.mod` through `go-version-file`, with `check-latest: false`. The container and CI then use the same version, 1.27.3. CI takes the Go archive that `setup-go` provides, not the digest-pinned container image. The version is the same, the source of the binary differs.
- age with `go install filippo.io/age/cmd/age@v1.2.1`, as in the Dockerfile. The Go checksum database verifies it.
- No other action and no other download.

### D4: Least privilege

- `permissions: contents: read` at workflow level.
- `actions/checkout` with `persist-credentials: false`, so the token is not left in the git configuration of the runner.
- No secrets. The trigger is `pull_request`, never `pull_request_target`.
- No cache. `setup-go` runs with `cache: false`. Downloading the three modules takes seconds, and a cache shared between runs could be filled by an earlier run.
- `timeout-minutes: 20` per job, and a concurrency group per branch that cancels a superseded run.

### D5: Triggers

Every push to any branch, every pull request and `workflow_dispatch`. The maintainer asked for every push. If runner minutes run short, macOS and Windows can be limited to the main branch and pull requests later. That is a change of the trigger only.

### D6: Test steps

On each runner, after checkout and Go setup:

1. Install age and print `age --version`, so the log shows the version.
2. `go vet ./...`.
3. `go test -count=1 -json ./...`, read by the skip report of D7.
4. `go test -count=1 -race ./...` with `CGO_ENABLED=1`, because the race detector needs cgo.

The plain run and the race run are kept apart. The plain run matches what developers run locally and what the binary is built as. The race run changes timing and needs a C compiler. The workflow sets `defaults.run.shell: bash`, because Windows would otherwise use PowerShell. With an explicit `bash`, GitHub runs steps with `-eo pipefail`, so a failing `go test` fails the step even behind a pipe.

### D7: Skip report

A small Go program in `internal/tools/skipreport` reads the `go test -json` stream from standard input. It:

- prints the test output as `go test` would, so the log stays readable,
- collects every skipped test with the output lines of that test, which hold the reason,
- writes a table of skipped tests to the job summary when `GITHUB_STEP_SUMMARY` is set, and to standard output otherwise,
- exits with status 1 when a test failed or when a test was skipped that the list of expected skips does not allow for the platform.

The expected skips are in `.github/expected-skips.txt`, one line per entry: platform (`linux`, `darwin`, `windows` or `*`), package, test name, and the reason after `#`. Initial content from the context above:

- `*`: `TestKillHelper` and `TestLeaveGenerationForOfflineExercise`.
- `windows`: `TestPermissionsOnLinux`, `TestSystemWithoutBW` and `TestPartialGenerationByAnotherPath`.

`TestRecoveryWithStandardTools` is not listed. It must run on every runner. An entry allows a skip but does not require it. Entries that did not skip are listed in the report as information.

The program uses only the standard library and has its own tests. It is not imported by any other package, so the command's dependency check is unaffected. Developers can run it locally the same way.

*Alternatives considered:* `gotestsum` needs a new dependency and does not check skips against a list. A text search over `go test -v` output depends on the shell and on the output format.

### D8: Checks job

On `ubuntu-24.04`:

- `test -z "$(gofmt -l .)"`, and on failure the list of files. Only on Linux, because a Windows checkout can convert line endings and gofmt would then report every file.
- `go mod tidy -diff`, which fails when `go.mod` or `go.sum` would change.
- The key-material search from the [test strategy](../../../docs/development/testing.md), as `git grep` with the same pattern. Its own workflow file is excluded besides `testing.md`, because both name the pattern. A match fails the job.

### D9: Failures found by the first run

- A test that relies on Linux behavior is fixed in the test or given a platform guard with a reason.
- A product defect is fixed here only if the fix is small and makes the code meet an existing spec on that platform. Each such fix gets a regression test and is listed in the change. An example would be a directory flush that macOS refuses with an error that means "not supported".
- Anything that needs a change of a spec or of a decision of an archived design is recorded in the test strategy and in the limitations, and goes to a follow-up change.
- No test is disabled or added to the expected skips without a reason that holds in review.

### D10: License

`LICENSE` holds the MIT license text with `Copyright (c) 2026 Georg Lutz`. The README names the license. `releases.md` records that the license is settled, and that the dependencies age, `golang.org/x/crypto` and `golang.org/x/sys` are under three-clause BSD licenses, whose notices a binary release must carry. Collecting those notices stays a release task.

*ADR check:* a license is hard to change once others contribute, but it changes neither an integration boundary nor a security assumption, and the `LICENSE` file is its authoritative record. CI is easy to change. Neither gets an ADR.

## Risks / Trade-offs

- [The Windows runner may lack a C compiler for the race detector] → The first run shows it. If so, the race step on Windows is dropped with a reason in the workflow and in the test strategy, and the plain test run stays.
- [Pinned actions do not receive fixes automatically] → `integrations.md` lists them with the other pins, to be reviewed together.
- [Antivirus scanning on Windows can hold files during rename] → The rename is retried on Windows. A flaky failure is recorded with its frequency and not hidden behind a retry of the test.
- [Runner minutes of private repositories count more for macOS and Windows than for Linux. The current factors were not checked for this design] → Usage is recorded after the first runs. D5 names the lever.
- [The private code runs on machines GitHub operates] → The repository holds only synthetic data and no secrets, and the workflow has read access only.
- [CI passing could be read as platform support] → The test strategy states what CI covers and what stays manual.

## Migration Plan

No migration. The maintainer creates a private repository, adds it as remote and pushes. The workflow starts on that push. Rollback is the removal of the workflow file.
