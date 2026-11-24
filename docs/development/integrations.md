# Integrations

The Dockerfile pins Debian-based Go and Node images by manifest digest. npm ci verifies the bw and OpenSpec packages against the integrity values in the lockfile. age and age-keygen come from the versioned Go module, verified by the Go checksum database. When updating, review pins and lockfile together, rebuild the image and repeat the relevant regression tests.

The unmodified bw version is deliberately pinned to the version used in the research. For later integration runs, use only isolated test profiles and synthetic accounts. Report Bitwarden Cloud and Vaultwarden separately. The live probe and the [manual login exercise](testing.md#manual-login-exercise) run Vaultwarden 1.37.4 inside the development container, bound to loopback with TLS, from an image fetched by digest without Docker. The port is forwarded to the host only, so that a browser and a `bw` on the host reach it. An instance outside the container remains possible for other runs.

The age library `filippo.io/age` is pinned in `go.mod` like every other module, and `go.sum` holds the checksums of the library and its dependencies. The age CLI is pinned separately in the Dockerfile and in `ci.yml`, at present to v1.3.2; keep these two equal. Library and CLI may differ in version. The recovery test opens archives written by the library with the CLI, so a difference tests what users meet, who open an archive later with whatever age they have.

## Dependency updates

Dependabot proposes version updates once a month as pull requests, one per ecosystem, configured in `.github/dependabot.yml`. A new release is proposed only after a cooldown of seven days. Review such a pull request like any other change of a pin, rebuild the dev container when it changes the Dockerfile, and update the pin table under [continuous integration](#continuous-integration) when it changes an action:

- **Go modules** in `go.mod`, the age library among them.
- **GitHub Actions** in both workflows, by commit SHA with the tag as comment, which `TestWorkflowStructure` checks.
- **Container images** of the Dockerfile: new digests of the same tag, and new minor and patch versions of Node. The Go version is not proposed, because it follows `go.mod`; update both by hand.

Not covered by version updates, and updated by hand: the Go version, the age CLI, the npm packages `@bitwarden/cli`, pinned to the researched version ([ADR 0007](../decisions/0007-bw-versions.md)), and `@fission-ai/openspec`, whose update also regenerates the skills with `openspec update`, and the runner labels. Dependabot alerts and security updates, enabled in the repository settings, still cover every dependency in `go.mod` and the npm lockfile. They do not cover the Go standard library.

Known advisories in the dev container, as of 24 November 2026 (`npm audit --package-lock-only` against the lockfile of the pin): the lockfile holds `node-forge` 1.4.0, `multer` 2.4.0 and `tmp` 0.0.33 through `@bitwarden/cli` 2026.11.1, with 24 advisories rated high or lower: 21 for `node-forge` (for example [GHSA-86w9-cpqp-85rv](https://github.com/advisories/GHSA-86w9-cpqp-85rv) and [GHSA-5cwg-6vw7-rj6p](https://github.com/advisories/GHSA-5cwg-6vw7-rj6p)), one for `multer` ([GHSA-q7qp-rjg7-q26m](https://github.com/advisories/GHSA-q7qp-rjg7-q26m), fixed in 2.5.0) and two for `tmp`, which reaches `bw` through `inquirer` and `external-editor` ([GHSA-52f5-9888-hmc6](https://github.com/advisories/GHSA-52f5-9888-hmc6), [GHSA-ph9p-34f9-6g65](https://github.com/advisories/GHSA-ph9p-34f9-6g65)). The move from 2026.10.1 removed the `multer` advisory [GHSA-3pph-fpjx-jg34](https://github.com/advisories/GHSA-3pph-fpjx-jg34). `npm audit` no longer reports `picomatch` 4.0.7 of `@fission-ai/openspec` 2.0.0, also not for the earlier lockfile, so the change lies in the advisory data and not in the lockfile. They stay, because 2026.11.1 is the newest `bw` and pins `node-forge` and `multer` exactly, and an npm override would modify `bw` against [ADR 0001](../decisions/0001-official-cli.md). The fixes that `npm audit` suggests are downgrades to older releases. The packages are used only in the dev container and are not part of bw-takeout. Whether `bw` reaches an affected code path of `node-forge` was not examined. Update the pins when a release with fixed dependencies appears.

Adding or updating a module needs a module cache that the container user can write to. The Dockerfile makes the content of `/go` belong to that user after the tools are installed. A container built before that change has a cache owned by root, so `go mod download` fails there until the image is rebuilt.

## Tested bw versions

The `bw` client of this project runs any `bw` version and produces no message about it ([ADR 0007](../decisions/0007-bw-versions.md)). This list is for developers only and is not shown to users. It records which version the project was tested with, on which platform and with which evidence level. Add a row when the pin of the dev container changes or a version is exercised on another platform.

| Version | Platform | Evidence |
|---|---|---|
| 2026.10.1 (npm package `@bitwarden/cli`) | Linux arm64, dev container | Code reading of the bundle, unauthenticated runs, live probe against Vaultwarden 1.37.4, manual login exercise from a Go parent against Vaultwarden 1.37.4 with five cases ([code notes](../research/bw-cli-code-notes-2026-11-15.md), [live probe](../research/vaultwarden-live-probe-2026-11-14.md), [login exercise](../research/login-exercise-2026-11-15.md)) |
| 2026.10.1 (official single-file program, `bw-macos-arm64`) | macOS 26.7.3 arm64 | Manual login exercise from a Go parent against Vaultwarden 1.37.4 in the dev container: successful login, wrong password and interrupt at the password prompt ([login exercise](../research/login-exercise-2026-11-15.md)) |
| 2026.10.1 | Windows | Not run |
| 2026.11.1 (npm package `@bitwarden/cli`), the pin since 24 November 2026 | Linux arm64, dev container | Code reading of the release source against 2026.10.1 and of the bundle, `bw --version` under Node 24 and 26, the automated Vaultwarden tests against Vaultwarden 1.37.4 with all scenarios ([code notes](../research/bw-cli-code-notes-2026-11-24.md), [test run](testing.md#vaultwarden-test-run-of-24-november-2026-bw-version-2026-11)) |
| 2026.11.1 | Linux amd64, `vaultwarden.yml` | The automated Vaultwarden tests in the dev container image built from the pin, on the branch of the change on 24 November 2026 ([CI run](testing.md#ci-run-of-24-november-2026-bw-version-2026-11)) |
| 2026.11.1 | macOS, Windows | Not run |

The automated [Vaultwarden tests](testing.md#vaultwarden-tests) ran with every scenario against 2026.10.1 and against 2026.11.1 from npm on Linux arm64 in the dev container on 24 November 2026, with the same findings and decryption error counts ([test run](testing.md#vaultwarden-test-run-of-24-november-2026-bw-version-2026-11)), and against 2026.11.1 on Linux amd64 through `vaultwarden.yml` on the branch of the change ([CI run](testing.md#ci-run-of-24-november-2026-bw-version-2026-11)). Before that, they ran against 2026.10.1 from npm on Linux arm64 in the dev container on 21 November 2026: an interactive login through `bw-takeout backup` at a pseudo-terminal, with the address, the master password and a two-step code, an intact vault and a tampered passkey, and the two login failures, a wrong master password and a wrong two-step code, against Vaultwarden 1.37.4. They ran in the same dev container image on Linux amd64 through `vaultwarden.yml` on `main` after the merge of the change ([CI run](testing.md#ci-run-of-21-november-2026-vaultwarden-test-setup)). The organization scenario, an admin backup with a chosen organization and a refused user backup, ran on Linux arm64 in the dev container and on Linux amd64 through `vaultwarden.yml` on the branch of the change organization-export on 21 November 2026 ([CI run](testing.md#ci-run-of-21-november-2026-organization-export)).

Bitwarden Cloud with an account is untested for every version.

## Vaultwarden test server

The Vaultwarden tests and the manual exercises use Vaultwarden 1.37.4, the image `docker.io/vaultwarden/server:1.37.4-alpine`, pinned by its image index digest `sha256:19ea2b669945d842dadc3d500f2d6a795866a003330f59a47b9634658bae98bb`. The index fixes the manifests and every blob. The table in `internal/e2e/fetch_test.go` repeats the manifest and the SHA-256 of the server binary per architecture, so that a mismatch is named:

| Architecture | Manifest | Server binary SHA-256 |
|---|---|---|
| arm64 | `sha256:c82bc74bb0a88821fd3fbce48e7867e8aca4e492e512970a309d02a629ab91d4` | `ae658947f85964e934d7673d9cab4c0edbce8e6167422d6ff3b10008e125421d` |
| amd64 | `sha256:7d90670a4e2531e752f014e58f7283b9d197847b6f073bf84e99598f49d80ffd` | `392bc9afa7de1821e372a6d93369c87c1070644c68e26057017a2bf1b9cac3be` |

The arm64 binary ran in the dev container. The amd64 binary was hashed in the [spike of 21 November 2026](../research/vaultwarden-registration-spike-2026-11-21.md) and first ran in the dispatched `vaultwarden.yml` run on `main` after the merge of the change.

To update Vaultwarden, fetch the index of the new release tag and its manifests as in step 1 of the [manual login exercise](testing.md#manual-login-exercise), unpack `vaultwarden` for each architecture, record its SHA-256 and check `vaultwarden --version` where it can run. Then replace the index digest, the version and both rows in `fetch_test.go`, in this page and in the manual exercise together, and run the Vaultwarden tests in the dev container and through `vaultwarden.yml`. A release can change the registration API or the prompts of the server, so the update is a reviewed change of its own.

## Continuous integration

`.github/workflows/ci.yml` and `.github/workflows/checks.yml` run on GitHub Actions. `ci.yml` holds the test matrix and runs only on changes to Go code, the module files, the list of expected skips or itself, to save runner minutes. `checks.yml` holds the checks that cover every file and runs on every change. Every input is pinned, and the pins are reviewed together with the pins above when they are updated:

| Input | Pin | Where |
|---|---|---|
| `actions/checkout` | `v7.0.1`, commit `3d3c42e5aac5ba805825da76410c181273ba90b1` | `uses:` with the tag as comment |
| `actions/setup-go` | `v7.0.0`, commit `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` | `uses:` with the tag as comment |
| Runner images | `ubuntu-24.04`, `macos-26`, `windows-2025` | matrix of the job `test` in `ci.yml`, and `runs-on` of the job `checks` in `checks.yml` |
| Go | the version in `go.mod`, through `go-version-file` | `setup-go` with `check-latest: false` |
| age CLI | `filippo.io/age/cmd/age@v1.3.2` | `go install`, the same version as the Dockerfile |

To update an action, resolve the commit of the new release tag with `git ls-remote https://github.com/actions/<name> 'refs/tags/<tag>' 'refs/tags/<tag>^{}'`, taking the peeled entry when both appear, and replace SHA and comment together. A versioned runner label fixes the operating system version, and it changes only by a commit. GitHub still updates the software preinstalled in the runner image without a commit, so a run can change result without one. Go and age are installed by the workflow and are not affected. The workflow has read-only access, uses no secrets and no cache, and runs on `pull_request`, never on `pull_request_target`. Results and the skip report are described in the [test strategy](testing.md#skip-report).

`.github/workflows/vaultwarden.yml` runs the [Vaultwarden tests](testing.md#vaultwarden-tests). Its only trigger is `workflow_dispatch`, so it starts by hand and never blocks a merge. It has one job on `ubuntu-24.04` with a limit of 30 minutes. It checks out with the same pinned `actions/checkout` as the other workflows, builds the dev container image from `.devcontainer`, and runs the suite inside it with the runner's user and group and with `HOME`, `GOCACHE` and `GOMODCACHE` under `/tmp`. It does not use `setup-go`. Go, Node, `bw` and age come from the pins of the Dockerfile, the Vaultwarden server from the [pins above](#vaultwarden-test-server). The job contacts Docker Hub and `https://api.bitwarden.com`, uses no secrets and no cache, and uploads nothing. `checks.yml` runs `go vet -tags vaultwarden ./...`, so the tagged code is checked on every change.
