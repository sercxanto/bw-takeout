# Proposal

## Why

The [requirements](../../../docs/product/requirements.md) say that the first prototype handles one account interactively with TOTP. The automated Vaultwarden suite of `vaultwarden-test-setup` only logs in with an address and a password. So no automated test covers the case the product is built for, and the only evidence of a login with two-step login is the manual login exercise and backup exercise of 15 and 18 November 2026. The login failure cases of `bw-process-adapter` (wrong password, and a wrong two-step code, which was never tried) have not run against a server since that exercise. The [architecture roadmap](../../../docs/development/architecture.md#roadmap), item 5, names this second change once `vaultwarden.yml` has passed on Linux amd64. It passed on 21 November 2026.

The explore session of 21 November 2026 narrowed the roadmap item to the login through the real binary. A deleted or corrupted attachment file on the server and an interrupt during a live backup move to a third change.

## What Changes

- The account helper of `internal/e2e` enables two-step login with an authenticator (TOTP) through the Vaultwarden API, and computes TOTP codes itself (RFC 6238, SHA-1, 30-second steps, 6 digits) with the Go standard library.
- The pseudo-terminal reader can compute an answer when the prompt appears, so it can type a TOTP code that is valid at that moment. Fixed answers keep working.
- **Intact vault** (`TestIntactVaultIsVerified`) runs with two-step login enabled after seeding. `bw` shows a third prompt for the code, and every existing assertion stays.
- **Tampered passkey** (`TestTamperedPasskeyFails`) stays an account without two-step login, so that both login paths stay covered.
- Two new scenarios on an account with two-step login and an empty vault:
  - **Wrong master password**: the backup ends with exit status 1, no generation, a failed login, an attempted logout, a removed profile and an empty profile location. `bw` asks for the address and the password only, never for the code.
  - **Wrong two-step code**: the same outcome after the code prompt, with a code that is invalid by construction.
- In both failure scenarios the test requires that a logout was attempted and that the profile was removed, as the `backup-run` spec requires. It does not pin the result that the `Logout:` line shows. The first run records that result. If the report says `failed` after a refused login, the architecture roadmap gets a follow-up for a clearer report. That follow-up is a separate change, because it would change the `backup-run` spec.
- Update the test strategy, the integration notes and the architecture roadmap.

Out of scope: a deleted or corrupted attachment file on the server, an interrupt during a live backup (including Ctrl-C at the code prompt), other two-step methods (e-mail, WebAuthn, Duo), more than one two-step method on one account, the "remember me" token of the server, Bitwarden Cloud, macOS and Windows.

This change adds no product feature and changes no command.

## Capabilities

### New Capabilities

None. The change adds test code and documentation. It sets `skip_specs: true`, because no behavior of the program changes. The acceptance criteria of the scenarios are in the design and the tasks.

### Modified Capabilities

None. The failure scenarios give the first automated evidence for the `backup-run` requirement "Logout after every login attempt" against a server. A report change that the first run may suggest is a separate change.

## Impact

**Product requirements addressed** ([requirements](../../../docs/product/requirements.md)): the first prototype handles one account interactively with TOTP. This change adds its first automated end-to-end evidence. It also adds evidence for "aborted runs must not leave a seemingly successful result" on a refused login.

**Specs touched without a delta:** `backup-run` ("Logout after every login attempt", "Exit status of a backup", "Report holds no vault content and no bw text") and `bw-invocation` (the interactive login). Their behavior is exercised, not changed.

**Code:** `internal/e2e` only, all behind the build tag `vaultwarden`. That means the account helper (enrollment and TOTP), the pseudo-terminal reader (computed answers), the backup call (answers as a parameter), the prompt child in `internal/e2e/testdata/promptchild` (a code prompt), and the scenarios. There is no change to `cmd/bw-takeout`, to any product package, or to the workflows.

**Dependencies:** none new. TOTP needs `crypto/hmac`, `crypto/sha1` and `encoding/base32` from the standard library.

**External contact during a run:** unchanged: Docker Hub and `https://api.bitwarden.com` once for each fresh `bw` profile. The two new scenarios start two more servers and two more fresh profiles, so they make two more of these requests.

**Documentation to update:**

- `docs/development/testing.md`: the Vaultwarden section (the third prompt, the new tests and scenarios, the list of what is not covered, and that the manual exercises stay for macOS and interrupts only), the sentence in "bw Process and Profile Tests" that says the login failure cases are not automated, and a record of the first run
- `docs/development/integrations.md`: the sentence on the automated runs against `bw` 2026.10.1
- `docs/development/architecture.md`: item 5 of the roadmap, with this change done, the third change for the attachment file and the interrupt, and the report follow-up if the first run calls for it
- `docs/user/limitations.md`: only if a run contradicts a statement there

**Existing ADRs:**

- [0001 Official bw CLI](../../../docs/decisions/0001-official-cli.md), accepted: TOTP is a public standard and no Bitwarden protocol. The test computes it to play the person at the terminal, never inside the product. No change.
- [0006 Secret boundary](../../../docs/decisions/0006-secret-boundary.md), proposed: the product still never sees the password or the code. The test types both at the terminal that `bw` reads. No change.
- [0007 Handling of bw versions](../../../docs/decisions/0007-bw-versions.md), proposed: the suite now also notices when a new `bw` changes the two-step prompt. No change.
- 0002, 0003, 0004 and 0005 are not touched.

**Open evidence:**

- The exact text of the two-step prompt of `bw` 2026.10.1. It is expected as `Two-step login code` and has not been observed at a pseudo-terminal.
- Whether `bw` exits after a wrong code or asks again. It exited without asking again after a wrong password in the login exercise. A wrong code was never tried.
- The failure class and the `Logout:` line after a refused login against a server. These are unobserved: the login tool logged out after failed logins only from a review fix that came after the exercise runs.
- The field names of the body of `POST /api/two-factor/get-authenticator` in Vaultwarden 1.37.4. Its struct is defined outside the file that was read in the explore session.
- Vaultwarden rejects a code whose time step is not later than the last accepted one, and the activation records its step (code reading of `src/api/core/two_factor/authenticator.rs` at 1.37.4). This was not observed at run time.
- Bitwarden Cloud remains untested. Results are Vaultwarden evidence only.
