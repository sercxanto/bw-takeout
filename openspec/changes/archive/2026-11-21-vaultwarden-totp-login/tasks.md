# Tasks

## Recommended apply setup

Model Sonnet, effort medium. The change is small and test-only, and the design fixes every mechanism, so little open reasoning is left. The session still has to recognize the stop points of tasks 1.2, 3.1 and 4.2 instead of working around a deviation, and keep the output hygiene of design D6.

## 1. TOTP and enrollment

- [x] 1.1 Implement the TOTP function of design D1 in a tagged `_test.go` file of `internal/e2e`. Verify with a known-answer test that uses the SHA-1 vectors of RFC 6238, Appendix B (secret `12345678901234567890`, the 8-digit values reduced to their last 6 digits, the RFC named in the test), and with a test that a 32-character base32 key without padding decodes to 20 bytes and that a key of another length is refused.
- [x] 1.2 Implement the enrollment of design D2 on `apiSession`: `get-authenticator`, then `authenticator` with a code for the current step. The account then keeps the secret and the last used step. First check the field names of the `get-authenticator` body against the source of Vaultwarden 1.37.4 or the running server, and correct D2 if they are not `masterPasswordHash`. Verify with a test against a started server: a new password grant without a code then fails with "Two factor required." and `TwoFactorProviders` lists exactly `0`. Record the TOTP secret for the log check under the kind `totp secret` (design D6). If the server lists another provider set, or the activation is refused for a reason other than a field name, stop and ask the maintainer.
- [x] 1.3 Add rows for the tests of 1.1 and 1.2 to the table of the "Vaultwarden Tests" section of `docs/development/testing.md`. Verify that the test names in the table match the code.

## 2. Computed answers at the pseudo-terminal

- [x] 2.1 Change `runWithPrompts` to take answer functions with a helper for fixed text (design D3). The reader calls each function once, when its prompt first appears, and an error from a function fails the run and names only the prompt. Pass the answers to `runBackup` as a parameter, with the address and the password of the account as the default. Verify that the existing pty tests pass unchanged except for the type of the answers.
- [x] 2.2 Extend `testdata/promptchild` by a mode with a third prompt `Two-step login code`, and change its unknown-prompt mode to a prompt name that the suite never answers (design D3). Verify with a unit test: the answer function of the third prompt runs once and only after the first two prompts were answered, the child reports that the answer arrived, and the failure message of an answer function that returns an error names the prompt but holds no answer.
- [x] 2.3 Implement the code answer of design D3: the code of the step `max(current, last used + 1)`, after which that step becomes the last used step. Verify with a unit test using a fixed clock: in the step of the activation the next step is typed, in a later step the current one, and two calls in the same step give two different, increasing steps.
- [x] 2.4 Update the description of the reader in the "Vaultwarden Tests" section of `docs/development/testing.md`: answers are computed when the prompt appears, and the code follows the replay rule of the server. Update the row of the pty tests. Verify that the description matches design D3.

## 3. Intact vault with two-step login

- [x] 3.1 Enable two-step login in `TestIntactVaultIsVerified` after seeding and add the prompt assertion of design D4. Verify that it passes in the dev container on Linux arm64, and record the exact text of the code prompt and the step typed relative to the activation. If the text is not `Two-step login code`, update the answer key, the prompt child and design D4. If `bw` shows a method chooser or any other extra prompt, stop and ask the maintainer.
- [x] 3.2 Verify that `TestTamperedPasskeyFails` still passes on its account without two-step login, and that it showed only the prompts `Email address` and `Master password`.

## 4. Login failure scenarios

- [x] 4.1 Add `TestWrongPasswordFailsLogin` with the setup and the assertions of design D5, the limit `loginFailureLimit` of 90 seconds, and the wrong password recorded for the log check. Verify that it passes in the dev container, and record the class of the `Failed:` line, the `Logout:` line and the duration.
- [x] 4.2 Add `TestWrongTwoStepCodeFailsLogin` with the code of design D5 that is wrong by construction, and the same assertions and limit. Verify that it passes in the dev container, and record the class, the `Logout:` line and the duration. Verify also that it fails when the wrong code is replaced on purpose by the valid code of design D3 (revert afterwards). If `bw` asks again after the wrong password or the wrong code, stop as design D5 says: record the observation in the testing page and ask the maintainer.
- [x] 4.3 Document both scenarios in the "Vaultwarden Tests" section of `docs/development/testing.md`. Remove two-step login and the login failure cases of bw-process-adapter from "Not covered", and state that the manual exercises stay for macOS and the interrupt cases. Update the sentence in "bw Process and Profile Tests" that says the login failure cases are not automated yet. Verify with `grep -n "not automated yet" docs/development/testing.md` that the old sentence is gone.
- [x] 4.4 If the `Logout:` line of 4.1 or 4.2 reads `failed`, add a follow-up to item 5 of the roadmap in `docs/development/architecture.md`: report a logout after a refused login in a way that does not read as a second failure, a change of the `backup-run` report (design D5). If it does not, record in the testing page that no follow-up was needed. Verify that exactly one of the two is in the docs.

## 5. Roadmap, integrations and limitations

- [x] 5.1 Update item 5 of the roadmap in `docs/development/architecture.md`: the second Vaultwarden change is done with this change, and a third change takes the deleted or corrupted attachment file on the server and the interrupt during a live backup. Blob items, lapsed premium and the archive filter still cannot be produced on Vaultwarden 1.37.4. Verify that the old follow-up text for the second change is gone and the third change appears once.
- [x] 5.2 Update the sentence on the automated runs against `bw` 2026.10.1 in `docs/development/integrations.md` to name two-step login and the two failure cases. Verify that the link to the testing page resolves.
- [x] 5.3 Compare `docs/user/limitations.md` and `docs/user/usage.md` with the results of 3.1, 4.1 and 4.2. Change them only where a run contradicts a statement. Verify by recording in the testing page entry of 6.1 whether a change was needed.

## 6. Integration checks

- [x] 6.1 Run `gofmt -l .`, `go vet ./...`, `go vet -tags vaultwarden ./...`, `go mod tidy -diff`, `go test -race ./...`, `GOOS=darwin go vet ./...`, `GOOS=windows go vet ./...`, `go test -count=1 -tags vaultwarden -v ./internal/e2e/` and `openspec validate vaultwarden-totp-login --strict` in the dev container. Verify that all pass, that the log check of `TestMain` found none of the recorded values (including `totp secret` and both passwords), and that the key-material check of `checks.yml` passes on the working tree. Record the run in the testing page as earlier changes did, with the duration of the suite.

## Workflow follow-up

- Run the independent code and security reviews required by the apply guidance before reporting completion.
- After the maintainer commits and pushes, dispatch `vaultwarden.yml` on the branch and record the run on Linux amd64 in the testing page. Check that the regular `CI` and `Checks` runs pass and that the skip report lists no new skip.
- Archive the change after review. It has no spec delta to sync.
- If 4.4 added the report follow-up, take it up in a later explore session, together with the third Vaultwarden change.
