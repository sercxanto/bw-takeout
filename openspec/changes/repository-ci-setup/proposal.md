# Proposal

## Why

Every test so far ran in the dev container on Linux arm64. The publishing code depends on file system behavior that differs on macOS and Windows: directory rename, exclusive creation, flushing and permissions. [Testing](../../../docs/development/testing.md) lists macOS, Windows and Linux amd64 as not run. Running the tests by hand on each platform for every change does not scale. The maintainer decided on 15 November 2026 to host the repository privately on GitHub, to test there with GitHub Actions, and to license the project under MIT. The README and the release notes ask for the license to be settled before a first release.

## What Changes

- Add a `LICENSE` file with the MIT license. The copyright holder is the git author of the repository, Georg Lutz, to be confirmed in review. Update the README and the release notes.
- Add one GitHub Actions workflow that runs on every push, every pull request and on manual start, on Linux amd64, macOS arm64 and Windows amd64:
  - `go vet ./...`, `go test ./...` and `go test -race ./...` on all three,
  - formatting, module tidiness and the key-material search on Linux.
- Install the pinned age CLI on every runner, so that the recovery test with the age and tar commands runs instead of being skipped.
- Make skipped tests visible: each run lists every skipped test with its reason in the job summary and fails on a skip that is not expected for that platform.
- Pin every input of the workflow: actions by full commit SHA, runner images by versioned label, Go by the version in `go.mod`, age by its module version. Grant the workflow read-only access, use no secrets and no caches.
- Record the first CI run and how to read CI results in the test strategy and the integration notes.
- Fix what the first run reveals on macOS or Windows: in tests, and in product code where a small fix makes the code meet the existing specs on that platform. A failure that needs a change of a spec or of a decision of an archived design is recorded and handed to a follow-up change.

This change adds no product feature and changes no command. Creating the GitHub repository, pushing and starting the first run are maintainer steps. The repository stays private.

## Capabilities

### New Capabilities

None. The change adds development tooling and documentation. It sets `skip_specs: true`, because no behavior of the program changes. The acceptance criteria for the workflow are in the design and the tasks.

### Modified Capabilities

None.

## Impact

**Product requirements addressed** ([requirements](../../../docs/product/requirements.md)): none directly. The change provides the platform evidence that the existing requirements on concurrent runs, aborted runs and offline readability need for macOS and Windows.

**Code:** new `.github/workflows/ci.yml`, a small Go program that reads the test output and reports skips, and a list of expected skips per platform. No change to the packages under `internal/` or to `cmd/bw-takeout`, except the fixes the first run makes necessary as described above.

**Development environment:** unchanged. The dev container stays the place to develop. CI adds three platforms and does not replace local test runs.

**Documentation to update:**

- `LICENSE` (new), `README.md` (license, CI)
- `docs/development/releases.md`: license settled, third-party licenses noted
- `docs/development/testing.md`: CI, expected skips, first CI run
- `docs/development/integrations.md`: pinned actions and runner images, how to update them
- `docs/development/getting-started.md`: where CI results are found
- `CONTRIBUTING.md`: a PR names its CI run

**Existing ADRs:**

- [0002 Go](../../../docs/decisions/0002-go.md), proposed: CI gives the first evidence for Windows and macOS. The ADR is not changed here. Its open point on interactive process handling stays open until the process adapter exists.
- [0005 Archive format and generation layout](../../../docs/decisions/0005-archive-format.md), proposed: its consequences on Windows flushing are tested on a CI runner for the first time. No change.
- 0001, 0003 and 0004 are not touched.

**Open evidence:**

- CI runners use NTFS on Windows and APFS on macOS. FAT and exFAT media, network shares and the manual offline exercise on a second system stay manual steps.
- GitHub-hosted runners are virtual machines operated by GitHub. Only synthetic data enters the repository and the runs, and the workflow holds no secrets.
- Runner minutes of private repositories count more for macOS and Windows than for Linux. The actual usage per run is unknown until the first run.
- The race detector needs a C compiler. Whether the Windows runner provides one in the expected way is unverified.
