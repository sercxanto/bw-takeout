# Getting Started with Explore

1. Open the repository folder containing the top-level README in VS Code. Docker must be running and the Dev Containers extension installed.
2. Choose **Dev Containers: Reopen in Container**. The first build needs network access. The personal VS Code feature provides Claude Code.
3. In the container terminal, inside the repository, run `go version`, `bw --version`, `age --version`, `openspec --version` and `openspec list`.
4. Start `claude`. Input: `/openspec-explore Read docs/development/first-explore-prompt.md and work on the brief.`
5. Review the results and choose a scope. Only then run, for example, `/openspec-propose Plan a synthetic regression test for attachments with the same name but different IDs.` Review the proposal, design, spec delta and tasks under openspec/changes/. Run `openspec validate <change-name> --strict` before the next step.

## Checking the Go code

Run these commands in the container terminal at the repository root:

```sh
gofmt -l .
go vet ./...
go test ./...
```

`gofmt -l .` prints the files that need formatting and prints nothing when all files are formatted. The tests need no network, no login and no `bw` installation. A few tests start the built command, the `age` and `tar` commands, a helper copy of the test binary or a fake `bw` that the tests build from this repository. Run `go test -race ./...` as well before a review. The [test strategy](testing.md) describes the fixtures and their evidence level.

### Tests with the real bw

The tests `TestRealBW…` start the installed `bw` without a login. They report a skip unless `BW_TAKEOUT_REAL_BW` is set:

```sh
BW_TAKEOUT_REAL_BW="$(command -v bw)" go test -count=1 -run TestRealBW -v ./internal/bwcli/
```

Each of them contacts Bitwarden Cloud once with an anonymous configuration request, so run them only when that is acceptable. See [bw process and profile tests](testing.md#bw-process-and-profile-tests).

## CI results

Once the repository is on GitHub, the workflow `CI` runs the tests on Linux amd64, macOS arm64 and Windows amd64 for a pull request and a push to `main`, but only when Go code, `go.mod`, `go.sum`, `.github/expected-skips.txt` or the workflow itself changed. The race tests run on the Linux runner only. The workflow `Checks` runs formatting, module tidiness and the key-material search on one Linux runner for every pull request and push to `main`, whatever changed. To test a branch without a pull request, start `CI` by hand with **Run workflow**, which ignores the path filter. Open the run under the **Actions** tab. The job `test` has one run per platform. Its summary lists every skipped test with its reason, and the run fails on a skip that is not expected for that platform, see the [skip report](testing.md#skip-report) and the [test strategy](testing.md). The pins of the workflow are in [integrations](integrations.md#continuous-integration).

## Building and running the command

```sh
go build -o .local/bin/bw-takeout ./cmd/bw-takeout
.local/bin/bw-takeout verify --identity <identity-file> <generation-directory>
```

The command has one subcommand so far, the offline check of a backup generation, described in [usage](../user/usage.md). `.local/` is ignored by git. A synthetic generation to try it on can be left by an opt-in test, see [testing](testing.md#archive-and-publishing-tests).

## OpenSpec setup

OpenSpec was initialized with `openspec init --tools claude,agents`. The generated skills are checked in. Further clients can be connected as needed following the [official CLI reference](https://github.com/Fission-AI/OpenSpec/blob/v2.0.0/docs/cli.md).
