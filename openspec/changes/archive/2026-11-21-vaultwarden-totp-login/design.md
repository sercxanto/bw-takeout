# Design

## Context

See [proposal.md](proposal.md) for motivation and scope. The explore session of 21 November 2026 settled the scope and read the TOTP code of Vaultwarden 1.37.4. The design builds on this state:

- **Suite.** `internal/e2e` (change `vaultwarden-test-setup`, archived on 21 November 2026) starts one Vaultwarden 1.37.4 per scenario, registers an account over the HTTP API, seeds it through the official `bw` with `--passwordenv`, and runs the built `bw-takeout backup` under a pseudo-terminal. `runWithPrompts` in [pty_test.go](../../../internal/e2e/pty_test.go) takes a `map[string]string` of fixed answers and answers each known prompt once. A prompt line drawn again after its answer is skipped as a redraw. An unknown prompt or the limit of 3 minutes (`backupLimit`) fails the run and names only the prompts seen. `account.login` in [account_test.go](../../../internal/e2e/account_test.go) gets an access token with the password grant and no two-step fields.
- **Vaultwarden 1.37.4, by code reading** (`src/api/core/two_factor/authenticator.rs` and `src/api/identity.rs` at the tag):
  - `POST /api/two-factor/get-authenticator` with the master password hash (or an OTP) returns `{enabled, key, object}`. The key is 20 random bytes in base32.
  - `POST /api/two-factor/authenticator` with `{key, token, masterPasswordHash}` checks that the key decodes to 20 bytes, validates `token` as a TOTP code for that key and enables the method.
  - The codes use SHA-1, 30-second steps and 6 digits. By default the server accepts the previous, current and next step.
  - A code is accepted only if its step is later than `last_used`, and every accepted code, the code of the activation included, sets `last_used` to its step.
  - A password grant without a code for an account with two-step login fails with `error_description` "Two factor required." and lists the enabled providers in `TwoFactorProviders`. With one provider, that provider is selected by default.
- **The login of the product.** `bwcli.Login` connects `bw login --raw` directly to the terminal ([bw-process-adapter design D6](../archive/2026-11-15-bw-process-adapter/design.md)). `bw` asks for the two-step code on that terminal, and the product never sees it. The login exercise of 15 November 2026 entered a code by hand. It saw a wrong password fail with the class `exit` and no second prompt. It never tried a wrong code.
- **Teardown.** The `backup-run` spec requires a logout after every login attempt and the removal of the profile on every path. The report prints `Failed:      login: …`, a `Logout:` line and a `Profile:` line ([report.go](../../../internal/report/report.go)). No run has shown which result the logout gives after a login that the server refused.

## Goals / Non-Goals

**Goals:**

- The product's main login case, address, password and TOTP code at the terminal, runs automatically through the built binary.
- Every code that the test types is valid or invalid by construction, never by chance, and no test waits for a step boundary.
- The failure scenarios prove what the spec promises after a refused login, and record what it does not specify.

**Non-Goals:**

- A second two-step login inside one test, the remember token of the server, recovery codes and the disabling of the method.
- Any change to the product, the report or a spec. The proposal names the follow-up for the `Logout:` line.
- Sending two-step fields from the test's own password grant. No step after the enrollment needs the API.

## Decisions

### D1: TOTP is computed in the test with the standard library

A function computes the code for a base32 secret and a step: HMAC-SHA1 over the step as an 8-byte big-endian number, dynamic truncation as in RFC 4226, and the result modulo 10^6, padded to 6 digits. It lives in a `_test.go` file of `internal/e2e` like everything else there.

A known-answer test uses the SHA-1 vectors of RFC 6238, Appendix B, with the secret `12345678901234567890` in ASCII. The RFC gives 8 digits. A 6-digit code is the same truncated number modulo 10^6, so the expected values are the last 6 digits of the RFC values. For example, `94287082` at T = 59 becomes `287082`. The test names the RFC and says so. A second test checks the base32 decoding of a key in the form the server returns: 32 characters without padding, decoding to 20 bytes.

The secret and every code exist in memory only.

*Alternative considered:* a TOTP package such as `pquerna/otp`. Rejected, because about 20 lines of the standard library do the job and the published vectors check them.

ADR 0001 is not affected: TOTP is a public standard, not a Bitwarden protocol or cryptography of its own, and the test computes it to play the person at the terminal, never inside the program.

### D2: Two-step login is enabled through the API after seeding

A method on `apiSession` enables the authenticator in two calls: `get-authenticator`, then `authenticator` with the returned key, a code for the current step and the master password hash of the account. The account then keeps the secret and the step of that code as its last used step.

After the activation the test checks that the method is on, the same way `tamperPasskey` reads the cipher back. A new password grant without a code must fail with "Two factor required." and list exactly the authenticator in `TwoFactorProviders`, which is provider `0` in Vaultwarden. A single provider means that `bw` shows no method chooser before the code prompt.

The order of every scenario with two-step login is: register, seed (intact scenario only), enable, back up. Seeding and the tampering of the passkey keep their password-only path, so `seedVault`, `account.login` and `tamperPasskey` do not change.

The body of `get-authenticator` is a `PasswordOrOtpData` struct defined outside the file that was read. The task that implements D2 checks its field names against the source of 1.37.4 or against the running server first. The expected name is `masterPasswordHash`, as in the activation body.

*Alternative considered:* enabling two-step login before seeding, so that the order matches a real account. Rejected, because the seed `bw` and the token request of the tampering would both need codes, and a third code would have to be fitted around the replay rule. The vault content does not depend on when two-step login was turned on.

### D3: A code is computed when the prompt appears

`runWithPrompts` takes a map from prompt name to answer, where an answer is a function that returns the text to type, or an error. A helper turns a fixed string into such a function, so the existing answers keep their form. The reader calls the function once, when it first sees the prompt. An error fails the run like an unknown prompt and names only the prompt.

For the code prompt, the account answers with the code of the step `max(current step, last used step + 1)` and records that step as its last used step. When the prompt appears in the same step as the activation, the code of the next step is typed. The server accepts it because it allows one step of drift, and the replay rule is met. The test never waits for a boundary. The time between the prompt and the server's check is a few seconds, well within the window of one step either way.

The prompt child in `testdata/promptchild` gets a mode that asks for a third prompt and reports whether the answer arrived. Its unknown-prompt mode names a prompt that the suite never answers. It uses `Two-step login code` now, which becomes a known prompt. A unit test checks that the answer function runs once, only after the prompts before it were answered, and that its text arrives in the child.

*Alternative considered:* waiting for the next step boundary after the activation and passing the code as a fixed answer. Rejected, because it adds up to 30 seconds per scenario, and a slow start of `bw` under Node could move the prompt into yet another step.

### D4: The intact scenario uses two-step login, the tampered one does not

`TestIntactVaultIsVerified` enables two-step login after seeding. It keeps every assertion of the `vaultwarden-test-setup` design D8 and adds one: `bw` showed exactly the prompts `Email address`, `Master password` and the code prompt, in this order.

`TestTamperedPasskeyFails` stays on an account without two-step login. Both login paths of `bw`, with and without the code prompt, then stay covered by a passing backup without a third scenario.

The first run records the exact text of the code prompt. If it is not `Two-step login code`, the implementer updates the answer key, this decision and the prompt child, as `vaultwarden-test-setup` did for the first two prompts. This needs no maintainer decision.

### D5: Two login failure scenarios

Both scenarios register an account, enable two-step login and run the backup. They seed no vault, because the run ends before the first document, and this saves about 45 seconds each.

- **Wrong master password** (`TestWrongPasswordFailsLogin`): the answer at `Master password` is a second random password of the same form, recorded for the log check. `bw` must show exactly `Email address` and `Master password`. A code prompt here would mean that the server asked for the second factor before it checked the password.
- **Wrong two-step code** (`TestWrongTwoStepCodeFailsLogin`): the answer at the code prompt is the code of a step 100 steps (50 minutes) before the current one. Before it is typed, the test compares it with the codes of the three steps the server accepts and with the code of the last used step. If it equals one of them, the test takes the step before and compares again. The code is therefore wrong by construction. `bw` must show the three prompts in order.

Both assert:

- exit status 1,
- the report line `Generation:  none published`,
- a report line that starts with `Failed:      login:`. The class after it is logged but not asserted. The login exercise saw `exit` for a wrong password, and a wrong code is unobserved,
- a `Logout:` line that is not `not needed, no login was started`, so a logout was attempted,
- the report line `Profile:     removed`,
- an empty target and an existing, empty profile location.

The test does not pin which result the `Logout:` line shows (maintainer decision of 21 November 2026). `bw logout` is likely to fail when no account is logged in, and how it fails is `bw` behavior, not product behavior. The first run records the line in the testing page. If it reads `failed`, the report tells the user about a second error that did not happen. The architecture roadmap then gets a follow-up to report a logout after a refused login in a way that does not read as a failure. That follow-up changes the `backup-run` spec, so it is not part of this change.

The failure scenarios use a limit of 90 seconds (`loginFailureLimit`) instead of `backupLimit`. A failed run starts about five `bw` processes, each a few seconds under Node, plus the logout. If `bw` asks again after a wrong answer, the reader skips the second prompt as a redraw and the limit ends the run. It then fails, naming the prompts seen, after 90 seconds and not 3 minutes.

If `bw` asks again after a wrong code or a wrong password, the test is not changed to answer a second time. The implementer records the observation in the testing page and asks the maintainer. A second prompt changes what the scenario claims: the product would then wait for a person who is not there, which is a question for the product, not for the test.

*Alternative considered:* a fresh random 6-digit code as the wrong code. Rejected, because it matches one of the three accepted codes with a probability of about 3 in a million, and a test that fails that rarely cannot be diagnosed.

### D6: Output hygiene

- The TOTP secret is recorded for the log check of `TestMain` under the kind `totp secret`, and the wrong password under `password`, like the right one.
- Codes are never logged. They are not added to the log check, because a search for 6 digits would match timestamps, durations and IDs in the log. The only path to the terminal is the pseudo-terminal, whose text is kept in memory and never printed.
- The report is logged with the address replaced, as before. A failed report holds the phase and the class, which hold no secret.

### D7: Documentation and decisions

- The testing page: the reader answers three prompts, the table gets the new tests, the failure scenarios get their assertions, the list of what is not covered loses two-step login and the login failure cases, and the manual exercises stay for macOS and the interrupt cases. The sentence in "bw Process and Profile Tests" that the login failure cases are not automated is updated. The first run is recorded with the prompt text, the failure classes, the `Logout:` lines and the durations.
- The integration notes: the sentence on the automated runs against `bw` 2026.10.1 names two-step login and the two failure cases.
- The architecture roadmap, item 5: the second Vaultwarden change is done. A third change takes the deleted or corrupted attachment file and the interrupt during a live backup. The report follow-up of D5 is added if the first run calls for it.
- The limitations change only if a run contradicts them. The statement on Ctrl-C at a prompt is not touched, because no interrupt runs here.
- No new ADR. The change adds test code that can be removed without affecting the product. It moves no integration boundary and fixes no security assumption of the program. D1 records why ADR 0001 is not affected, and the product side of ADR 0006 does not change.

## Risks / Trade-offs

- [The body of `get-authenticator` has other field names than expected] → The task of D2 checks them first. A refusal names the server's message, which holds no secret.
- [The code prompt has another text, or `bw` shows a method chooser] → The reader names an unknown prompt. D4 allows a text update without a decision. A chooser contradicts the single-provider check of D2 and is a stop point.
- [`bw` asks again after a wrong answer] → The limit of D5 ends the run after 90 seconds, and D5 stops for a maintainer decision.
- [The server's replay rule or drift window differs from the code reading] → The intact scenario fails at the login with a failed login instead of a verdict, which is loud. The task of D3 records which step was typed relative to the activation, so the cause can be read from the log.
- [Rate limits on failed logins] → Each scenario has its own server and makes one failed attempt. Vaultwarden's defaults allow far more per minute.
- [Two more servers and profiles make the suite longer] → About 30 to 40 seconds each without seeding. The suite stays well within the 30 minutes of `vaultwarden.yml`.
- [The `Logout:` line is not pinned, so a regression in it goes unnoticed] → Accepted by the maintainer. The line is recorded per run, and the follow-up of D5 decides what it should say.
