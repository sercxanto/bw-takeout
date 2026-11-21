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

### Tests against Vaultwarden

The end-to-end tests in `internal/e2e` run the built command against a local Vaultwarden server with a synthetic account, with no person at the terminal. They are behind the build tag `vaultwarden` and run on Linux only:

```sh
go test -count=1 -tags vaultwarden -v ./internal/e2e/
```

A run takes about four minutes. It fetches the pinned Vaultwarden image from Docker Hub into `.local/vaultwarden-cache/` and contacts Bitwarden Cloud once for each fresh `bw` profile. See [Vaultwarden tests](testing.md#vaultwarden-tests).

## CI results

Once the repository is on GitHub, the workflow `CI` runs the tests on Linux amd64, macOS arm64 and Windows amd64 for a pull request and a push to `main`, but only when Go code, `go.mod`, `go.sum`, `.github/expected-skips.txt` or the workflow itself changed. The race tests run on the Linux runner only. The workflow `Checks` runs formatting, `go vet -tags vaultwarden ./...` for the [Vaultwarden tests](#tests-against-vaultwarden), module tidiness and the key-material search on one Linux runner for every pull request and push to `main`, whatever changed. To test a branch without a pull request, start `CI` by hand with **Run workflow**, which ignores the path filter. Open the run under the **Actions** tab. The job `test` has one run per platform. Its summary lists every skipped test with its reason, and the run fails on a skip that is not expected for that platform, see the [skip report](testing.md#skip-report) and the [test strategy](testing.md). The pins of the workflow are in [integrations](integrations.md#continuous-integration).

The workflow `Vaultwarden` runs the tests against Vaultwarden on Linux amd64. It never starts on its own: open **Actions**, choose **Vaultwarden** and start it with **Run workflow** on the branch or the release commit. It is required before a release, see [releases](releases.md).

## Building and running the command

```sh
go build -o .local/bin/bw-takeout ./cmd/bw-takeout
.local/bin/bw-takeout backup --server <https-url> --target <dir> --recipients-file <file>
.local/bin/bw-takeout verify --identity <identity-file> <generation-directory>
```

The command has two subcommands, the interactive backup of one account and the offline check of a backup generation, described in [usage](../user/usage.md). A backup needs a server with an account; the [manual backup exercise](testing.md#manual-backup-exercise) sets up a local Vaultwarden instance. `.local/` is ignored by git. A synthetic generation to try it on can be left by an opt-in test, see [testing](testing.md#archive-and-publishing-tests).

## OpenSpec setup

OpenSpec was initialized with `openspec init --tools claude,agents`. The generated skills are checked in. Further clients can be connected as needed following the [official CLI reference](https://github.com/Fission-AI/OpenSpec/blob/v2.0.0/docs/cli.md).
