# Tasks

## 1. Dev container repair and dependency

- [x] 1.1 Change `.devcontainer/Dockerfile` so that the content of `/go` belongs to the container user after the tools are installed (design D13); verify by reading the build steps that no later step writes to `/go` as root, and state in the task result that the running container is unchanged until the maintainer rebuilds it
- [x] 1.2 If `go mod download` cannot write to the module cache in the running container, set `GOPATH` to a directory under the ignored `.local/` for all Go commands of this session; verify `go env GOPATH` in the session and that `git status` shows no new tracked or untracked file from it
- [x] 1.3 Add `filippo.io/age v1.2.1` to `go.mod` and commit `go.sum`; verify `go mod tidy` leaves both files unchanged and `go build ./...` succeeds
- [x] 1.4 Record in `docs/development/integrations.md` that the age library and the age CLI are pinned to the same version and updated together, and that the module cache must be writable for the container user; verify the page names both pins

## 2. Manifest

- [x] 2.1 Implement `internal/manifest` with the keys of design D5, the format name and version, and the verification result as raw JSON; verify a test round-trips a manifest and shows the embedded result byte-identical to the supplied one
- [x] 2.2 Reject an unknown format version and ignore unknown additional keys when reading; verify one test for each
- [x] 2.3 Validate entries when reading a manifest: unique paths, non-negative sizes, SHA-256 in lowercase hexadecimal of full length, known kinds; verify a test per violation

## 3. Archive writer and reader

- [x] 3.1 Implement the writer in `internal/archive` on an `io.Writer`: `RECOVERY.txt` first, the three documents, attachments by ID, manifest last, tar entry attributes as in design D2, encryption to X25519 recipients; verify a test writes an archive from the intact fixture and reads it back with every entry byte-identical
- [x] 3.2 Validate item IDs as UUIDs and attachment IDs as 1 to 64 ASCII letters and digits before use in an entry name (design D3); verify tests accept the fixture's attachment IDs, refuse for item IDs and attachment IDs each an ID with a path separator, with a parent directory reference, with a wrong length and an empty one, and that the writer reports an error without finishing the archive
- [x] 3.3 Refuse writing without a recipient and with a recipient string that is not a native age recipient; verify the error names the position and not the value
- [x] 3.4 Implement the reader on an `io.Reader`: sequential, hashing entries as they pass, checks of the `offline-verification` spec for entry types, names, duplicates, manifest position and the comparison with the manifest; verify the scenarios "Entry differs from the manifest", "Entry not listed", "Listed entry missing", "Symbolic link entry" and "Newer format version" with archives built for the purpose
- [x] 3.5 Read the stream to its end before reporting success; verify the scenarios "Truncated archive file" at several cut positions, including inside the header and inside the last chunk, and "Altered byte" at several positions
- [x] 3.6 Verify the scenarios "Two recipients" and "Identity of another key pair" with key pairs generated at test time
- [x] 3.7 Write the text of `RECOVERY.txt` as a fixed string in the package; verify the scenario "Instructions present and free of vault data" and that the text names every top-level entry of the layout
- [x] 3.8 Verify the scenario "Temporary location stays empty" by pointing the temporary directory of the test process at an empty directory while writing and reading
- [x] 3.9 Add the recovery test with the `age` and `tar` commands from the search path (design D12); verify the scenario "Recovery with the age and tar commands" in the dev container and that the test reports a skip with a reason where a command is missing
- [x] 3.10 Write `docs/decisions/0005-archive-format.md` with status proposed, covering context, alternatives and consequences for design D2, D4, D5, D6 and D8, and add it to `docs/decisions/README.md`; verify the index lists it as proposed
- [x] 3.11 Update `docs/decisions/0003-age.md` as described in design D14 and keep its status proposed; verify it names the recipient model, multiple recipients against key loss, the tested offline recovery, and the two open points
- [x] 3.12 Review checkpoint: stop and show the maintainer the output of `go test -v ./internal/manifest/... ./internal/archive/...`, with every skipped test and its reason listed separately; verify the maintainer confirmed before any task of group 4 starts

## 4. Publishing

- [ ] 4.1 Implement `internal/publish` with the steps of design D8, injectable clock and random source, and the verdict rule of design D9; verify the scenarios "Verified capture", "Unverified capture" and "Failed capture"
- [ ] 4.2 Verify the scenarios "Two generations in the same second" and "Name collision" by forcing the clock and the random source, including a pre-existing final name and a pre-existing partial name
- [ ] 4.3 Verify the scenarios "Successful run next to older generations" and "Failing run next to older generations" by hashing every pre-existing file in the target before and after, with a failure injected at each step of design D8
- [ ] 4.4 Verify the scenario "Concurrent runs with identical time and random source" with eight concurrent publishers, and run the test repeatedly with the race detector
- [ ] 4.5 Verify the scenario "Write error in the middle" with a writer that fails after a number of bytes, including an out-of-space error, and that only the run's own partial directory is left or removed
- [ ] 4.6 Verify the scenario "Process killed while writing" with a helper process started from the test binary (design D12)
- [ ] 4.7 Implement the listing of partial generations with name and age; verify the scenario "Leftover from an aborted run" and that listing changes nothing in the target
- [ ] 4.8 Verify the scenarios "Missing target" and a target that is a regular file
- [ ] 4.9 Verify the scenario "Permissions on Linux", and skip the mode check with a reason on platforms without POSIX permissions
- [ ] 4.10 Flush the archive file and, where supported, the directories; verify by a test with an injected flush error that the generation stays partial, and document in `docs/user/limitations.md` what is not guaranteed on Windows and on FAT or exFAT media
- [ ] 4.11 Review checkpoint: stop and show the maintainer the output of `go test -v -race ./internal/publish/...`, with every skipped test and its reason listed separately, and state whether the kill test and the concurrency test actually ran; verify the maintainer confirmed before any task of group 5 starts

## 5. Verify command

- [ ] 5.1 Add `cmd/bitwarden-backup` with the subcommand `verify --identity <file> <generation-directory>` and the exit statuses of design D10; verify `go build ./cmd/...` and a usage error with status 2 for missing arguments
- [ ] 5.2 Verify the scenarios "Intact generation", "Verified capture", "Unverified capture" and "Failed verification" by running the built command against generations made from the fixtures
- [ ] 5.3 Verify the scenario "Partial generation" and a directory without an archive file
- [ ] 5.4 Verify the scenario "System without bw" by running the command with an empty search path, and verify with `go list -deps ./cmd/bitwarden-backup` that the network package is not among the dependencies
- [ ] 5.5 Verify the scenario "Generation with sentinel values" for standard output and standard error, for an intact generation and for one with a damaged entry
- [ ] 5.6 Refuse an identity file that cannot be parsed or holds no identity, without printing its content; verify a test with a file that holds a sentinel line
- [ ] 5.7 Write `docs/user/recovery.md`: verifying with the command, recovering with the age and tar commands alone, what to keep outside the archive (identity, age program, these steps), and the warning that manual unpacking writes plaintext to disk; verify every command on the page runs as written against a synthetic generation
- [ ] 5.8 Update `docs/user/usage.md` with the verify command and its exit statuses, `docs/user/security.md` with key separation, permissions and the missing proof of origin, and `docs/user/limitations.md` with what offline verification does and does not show; verify each statement corresponds to a test or to a risk in design.md

## 6. Integration check

- [ ] 6.1 Add an end-to-end test: fixture capture, verification, archive, publishing, then the verify command on the result, once for a verified and once for an unverified capture; verify both pass
- [ ] 6.2 Add the opt-in test that leaves a synthetic generation for the manual offline exercise (design D12); verify it does nothing without its environment variables and writes one verifiable generation with them
- [ ] 6.3 Cross-compile with `GOOS=darwin` and `GOOS=windows` for `./...` and run `go vet` for both; verify both succeed
- [ ] 6.4 Run `gofmt -l .`, `go vet ./...`, `go test ./...` and `go test -race ./...` in the dev container; verify no output from gofmt and all tests pass, and record platform, versions and date in `docs/development/testing.md` together with what was not run: macOS, Windows, FAT or exFAT media, network shares, the manual offline exercise on a second system, large attachments
- [ ] 6.5 Update `docs/development/architecture.md` (component status, roadmap step 2), `docs/development/getting-started.md` (build and verify commands) and the project status in `README.md`; verify the pages name the new packages and the command
- [ ] 6.6 Compare every scenario of the three specs with the test that covers it and add the mapping to `docs/development/testing.md`; verify the mapping is complete
- [ ] 6.7 Run `openspec validate archive-generation --strict`; verify it passes

## Workflow follow-up

- The maintainer rebuilds the dev container after task 1.1. Afterwards `go mod download` must work with the default `GOPATH`.
- Review scope, risks and acceptance criteria before implementing, as CONTRIBUTING requires.
- The manual offline exercise on a second system and test runs on macOS and Windows are the maintainer's steps. Add their results to the test strategy with platform and date.
- Before archiving, have the reader in `internal/archive` and the package `internal/publish` reviewed in a fresh session.
- Archive the change after review and verify the archived result.
