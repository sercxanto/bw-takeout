# Integrations

The Dockerfile pins Debian-based Go and Node images by manifest digest. npm ci verifies the bw and OpenSpec packages against the integrity values in the lockfile. age and age-keygen come from the versioned Go module, verified by the Go checksum database. When updating, review pins and lockfile together, rebuild the image and repeat the relevant regression tests.

The unmodified bw version is deliberately pinned to the version used in the research. For later integration runs, use only isolated test profiles and synthetic accounts. Report Bitwarden Cloud and Vaultwarden separately. Run a Vaultwarden test instance outside the development container if needed.

The age library `filippo.io/age` in `go.mod` and the age CLI installed by the Dockerfile are pinned to the same version, v1.2.1. Update both together, because the recovery test checks archives written by the library against the CLI. `go.sum` holds the checksums of the library and its dependencies.

Adding or updating a module needs a module cache that the container user can write to. The Dockerfile makes the content of `/go` belong to that user after the tools are installed. A container built before that change has a cache owned by root, so `go mod download` fails there until the image is rebuilt.

## Tested bw versions

The `bw` client of this project runs any `bw` version and produces no message about it ([ADR 0007](../decisions/0007-bw-versions.md)). This list is for developers only and is not shown to users. It records which version the project was tested with, on which platform and with which evidence level. Add a row when the pin of the dev container changes or a version is exercised on another platform.

| Version | Platform | Evidence |
|---|---|---|
| 2026.10.1 (npm package `@bitwarden/cli`) | Linux arm64, dev container | Code reading of the bundle, unauthenticated runs, live probe against Vaultwarden 1.37.4 ([code notes](../research/bw-cli-code-notes-2026-11-15.md), [live probe](../research/vaultwarden-live-probe-2026-11-14.md)) |
| 2026.10.1 | macOS | Not yet run. The manual login exercise of the change `bw-process-adapter` is planned |
| 2026.10.1 | Windows | Not run |

Bitwarden Cloud with an account is untested for every version.

## Continuous integration

`.github/workflows/ci.yml` runs on GitHub Actions. Every input is pinned, and the pins are reviewed together with the pins above when they are updated:

| Input | Pin | Where |
|---|---|---|
| `actions/checkout` | `v7.0.1`, commit `3d3c42e5aac5ba805825da76410c181273ba90b1` | `uses:` with the tag as comment |
| `actions/setup-go` | `v7.0.0`, commit `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` | `uses:` with the tag as comment |
| Runner images | `ubuntu-24.04`, `macos-26`, `windows-2025` | matrix of the job `test`, and `runs-on` of the job `checks` |
| Go | the version in `go.mod`, through `go-version-file` | `setup-go` with `check-latest: false` |
| age CLI | `filippo.io/age/cmd/age@v1.2.1` | `go install`, the same version as the Dockerfile and `go.mod` |

To update an action, resolve the commit of the new release tag with `git ls-remote https://github.com/actions/<name> 'refs/tags/<tag>' 'refs/tags/<tag>^{}'`, taking the peeled entry when both appear, and replace SHA and comment together. A versioned runner label changes only by a commit, so a run does not change result without one. The workflow has read-only access, uses no secrets and no cache, and runs on `pull_request`, never on `pull_request_target`. Results and the skip report are described in the [test strategy](testing.md#skip-report).
