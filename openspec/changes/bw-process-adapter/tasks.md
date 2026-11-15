# Tasks

## 1. Evidence and setup

- [x] 1.1 Write `docs/research/bw-cli-code-notes-2026-11-15.md` with the code-level and unauthenticated findings listed in the design context: version, bundle hash, method, each finding with its line number, and what was not tested; add a row to `docs/research/research-summary.md`; verify every finding of the design context appears in the note with its evidence level
- [x] 1.2 Add `golang.org/x/term` to `go.mod` and commit `go.sum` (design D17); verify `go mod tidy` leaves both files unchanged and `go build ./...` succeeds
- [x] 1.3 Add the third-party notice for `golang.org/x/term` to `docs/development/releases.md`; verify the page names the module and its license
- [x] 1.4 Move `ValidItemID` and `ValidAttachmentID` to `internal/vaultid` and let `internal/archive` use them (design D5); verify `go test ./internal/archive/...` passes unchanged and `go list -deps ./internal/vaultid` shows only standard library packages
- [x] 1.5 Add to the research note that `bw` has no error codes (32091, 31452); verify the note names both lines

## 2. Fake bw

- [x] 2.1 Implement `internal/bwcli/fakebw` as a `main` package that reads its scenario from the data directory and appends one record per start with arguments, environment, working directory and whether standard input was empty (design D12); verify a test builds it, runs it directly and reads the record
- [x] 2.2 Add the test helper that builds the fake once per package run into a temporary directory and writes scenario files; verify it works with `GOOS` set to the host and that `GOOS=windows go vet ./internal/bwcli/...` passes
- [x] 2.3 Implement the scenario behaviors of design D12, including exit code 0 when `BW_CLEANEXIT` is present and a failure when `BW_NOINTERACTION` is missing on a command other than login; verify one test per behavior against the fake alone, and that the package comment of the fake names the evidence for each behavior

## 3. Process core

- [x] 3.1 Implement the client configuration and the environment allowlist of design D3; verify the scenarios "Exit code override in the calling environment", "Runtime options in the calling environment", "Certificate authority file configured" and "Certificate authority file only in the calling environment" from the records of the fake
- [x] 3.2 Implement the command runner of design D4: empty standard input, output in memory with the limit of D8, time limit, process group on Unix; verify the scenarios "Command does not end", "Run is cancelled" and "Output exceeds the limit", and that the fake process no longer exists after a timeout on Unix
- [x] 3.3 Implement the failure type and classes of design D7 with a text built only from operation, class, exit code and IDs, the status query after a failed vault command, using the strict status parsing that task 4.3 exposes as an operation, and the premium sentence for the download only; verify the scenarios "Not logged in", "Locked vault", "Premium required", "Unrecognized error text", "Not-logged-in text with an unlocked vault" and "Status query after a failure fails", a premium sentence wrapped in colour codes, and the premium sentence on a command other than the download
- [x] 3.4 Scan standard error as a stream, count decryption error lines and keep nothing else; verify the scenarios "Export with decryption errors" and "Export without error lines", and that a standard error of several megabytes neither blocks the command nor is held in memory
- [x] 3.5 Check the working directory after every command; verify the scenarios "Export" and "bw writes a file"
- [x] 3.6 Verify the scenario "Failed download with a name in the error text" with a sentinel value in standard error and in standard output, searching the failure text and every exported field of the failure
- [x] 3.7 Review checkpoint: stop and show the maintainer the output of `go test -v ./internal/bwcli/...` with every skipped test and its reason; verify the maintainer confirmed before any task of group 4 starts

## 4. Operations

- [x] 4.1 Implement the version query and the SHA-256 of the resolved executable (design D5); verify the scenarios "Version query", "Version unknown to this project" and "Version output is empty", and a test where the executable is reached through a symbolic link on Unix
- [x] 4.2 Implement the check that `bw` uses the data directory; verify the scenario "bw uses another data directory" and that no login process appears in the records of the fake
- [x] 4.3 Implement the status operation with strict parsing; verify the scenarios "Status of a profile without login", "Status without a sync time", "Status document does not parse" and "Unknown status value", and a `lastSync` value that is not a time
- [x] 4.4 Implement the server configuration with the address check; verify the scenarios "Address without HTTPS" and "HTTPS address", and addresses with user information, without host and with a line break
- [x] 4.5 Implement sync, both exports and both item listings with the arguments of design D5; verify from the records of the fake that each call has exactly these arguments and that the documents are returned byte for byte
- [x] 4.6 Implement the attachment download with the ID rules of `internal/vaultid`; verify the scenarios "Download by ID pair" and "ID that starts with a dash", and an ID with a path separator, an empty ID and an ID of wrong length
- [x] 4.7 Implement the logout, which overwrites the session key whatever the outcome; verify the scenarios "Command before login", "Command after login", "Command after logout" and "Failure with a session key in use" from the records of the fake
- [x] 4.8 Write `docs/decisions/0007-bw-versions.md` with status proposed as described in design D16 and add it to `docs/decisions/README.md`; verify the index lists it as proposed and that it names the three alternatives and the consequences
- [x] 4.9 Add the internal list of tested `bw` versions to `docs/development/integrations.md` and state that it is not shown to users; verify it names 2026.10.1 with the evidence level per platform

## 5. Interactive login

- [x] 5.1 Implement the terminal interface with the implementation on `golang.org/x/term` and a fake for tests (design D6); verify `GOOS=darwin` and `GOOS=windows` builds
- [x] 5.2 Implement the login: terminal check first, inherited standard input and standard error, piped standard output, no `BW_NOINTERACTION`, no session key in the environment; verify the scenarios "Login without a terminal" and "Login with a terminal", the latter with a fake that prompts on standard error and reads a line from standard input
- [x] 5.3 Validate the form of the session key; verify the scenarios "Exit code 0 with empty output", "Valid session key", "Session key followed by a line ending", "Other text next to the key" and "Non-zero exit code with a key", and keys of 87 and 89 characters, with two line endings and with a wrong alphabet
- [x] 5.4 Save and restore the terminal state and hold interrupt signals while the child runs; verify the scenario "Interrupt at the password prompt" with the fake terminal and a fake that ends on a signal, and that the state is restored also when the login fails for another reason
- [x] 5.5 Write `docs/decisions/0006-secret-boundary.md` with status proposed as described in design D15 and add it to `docs/decisions/README.md`; verify the index lists it as proposed and that it names the four alternatives and the three consequences
- [x] 5.6 Review checkpoint: stop and show the maintainer the output of `go test -v -race ./internal/bwcli/...` with every skipped test and its reason; verify the maintainer confirmed before any task of group 6 starts

## 6. Profile

- [x] 6.1 Implement `internal/profile` with creation as in design D9, injectable location, clock and random source; verify the scenarios "Two runs in the same second", "Chosen name already exists", "Profile location is missing" and "New profile"
- [x] 6.2 Verify the scenario "Permissions on Linux and macOS" and skip the mode check with a reason on Windows; add the skip to `.github/expected-skips.txt` for Windows with the reason
- [x] 6.3 Implement teardown with the logout as a function; verify the scenarios "Teardown after a successful run", "Logout fails", "Removal fails" with an injected removal error, and "Teardown without login"
- [x] 6.4 Implement the listing of leftovers; verify the scenarios "Leftover from a killed run", "Own profile" and "Unrelated entries in the profile location", hashing every pre-existing file before and after
- [x] 6.5 Verify with `go list -deps ./internal/profile` that the package imports neither `internal/bwcli` nor `os/exec`
- [x] 6.6 Update `docs/user/security.md` (session key, profile content, leftovers and how to remove them) and `docs/user/limitations.md` (one device entry per run, the Cloud contact also for self-hosted servers, the premium requirement as code-level evidence, no proxy support yet); verify each statement corresponds to a finding in the research note or a risk in design.md

## 7. Tests with the real bw

- [x] 7.1 Add the tests `TestRealBW…` of design D13, which run only when `BW_TAKEOUT_REAL_BW` holds a path; verify they report a skip with the reason without the variable, and add them to `.github/expected-skips.txt` for every platform with the reason
- [x] 7.2 Run them in the dev container with `BW_TAKEOUT_REAL_BW="$(command -v bw)"`; verify all pass, and compare each result with the fake behavior it corresponds to
- [x] 7.3 Describe the fake, the switch, the Cloud contact of these tests and how to run them in `docs/development/testing.md` and `docs/development/getting-started.md`; verify the documented command runs as written

## 8. Manual login exercise

- [ ] 8.1 Implement `internal/tools/loginprobe` as described in design D14; verify it builds for linux, darwin and windows, and verify against the fake that its output contains neither the session key nor any text the fake wrote
- [ ] 8.2 Write the procedure of design D14 into `docs/development/testing.md`: starting Vaultwarden with TLS in the dev container, forwarding the port, the account with TOTP, the five cases for the dev container, the two cases for macOS and what to record; update the sentence on the Vaultwarden instance in `docs/development/integrations.md`; verify every command of the procedure was run as written in the dev container up to the point where an account is needed
- [ ] 8.3 Review checkpoint: stop and ask the maintainer to run the five cases in a terminal of the dev container first, and then the successful login and the interrupt at the password prompt on macOS; verify the maintainer reported the result of each of the seven runs, and do not ask for the macOS runs when the login fails in the dev container
- [ ] 8.4 Write the results as a report under `docs/research/` with versions, method, scope and what was not tested, including what the logout left in the profile, and add a row to the research overview; verify the report separates Linux and macOS, states which cases ran on which system, and names Bitwarden Cloud and Windows as not tested
- [ ] 8.5 Update `docs/decisions/0002-go.md` with the evidence and the open Windows point and keep its status proposed; verify the index still lists it as proposed

## 9. Integration check

- [ ] 9.1 Run `gofmt -l .`, `go vet ./...`, `go test -count=1 -json ./... | go run ./internal/tools/skipreport` and `go test -race ./...` in the dev container; verify no output from gofmt, all tests pass and every skip is on the list of expected skips
- [ ] 9.2 Cross-compile with `GOOS=darwin` and `GOOS=windows` for `./...` and run `go vet` for both; verify both succeed
- [ ] 9.3 Compare every scenario of the two specs with the test that covers it and add the mapping to `docs/development/testing.md`, together with the test run, its platform, versions and date and what was not run; verify the mapping is complete
- [ ] 9.4 Update `docs/development/architecture.md` (profile per run as a maintainer decision for the prototype, `bwcli` reports version and hash, component status, roadmap step 3 done, the follow-ups of the proposal in the roadmap, the two new ADRs) and the project status in `README.md`; verify the page no longer states "check version and hash" and names every follow-up of the proposal
- [ ] 9.5 Run `openspec validate bw-process-adapter --strict`; verify it passes

## Workflow follow-up

- Review scope, risks and acceptance criteria before implementing, as CONTRIBUTING requires.
- Module downloads for task 1.2 need network access in the dev container.
- The first CI run after the implementation shows the fake-based tests on Linux amd64, macOS and Windows. Record it in the test strategy as for the earlier changes.
- Tasks 8.3 to 8.5 depend on the maintainer's manual runs. If the login fails in the dev container or on macOS, stop and reopen design D6 before continuing.
- Before archiving, have `internal/bwcli` reviewed in a fresh session, with attention to the environment allowlist and to every place where CLI text could reach a result.
- Archive the change after review and verify the archived result.
