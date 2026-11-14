# Completeness Verdict and Scope Claim

Status: proposed

## Context

The [completeness experiments](../research/completeness-decision-2026-10-31.md) reproduced at SDK and export-service level that tolerant decryption can return an item as successful while all its passkeys are removed. The CLI then reports success. Exit codes and matching item counts do not prove completeness. No public interface offers an authoritative server inventory to compare a capture with. A backup that calls itself complete without evidence would mislead the user exactly in the case the product exists for.

## Decision

Every capture gets exactly one of three verdicts:

- **failed**: a proven deviation or broken input,
- **unverified**: a required check was not possible,
- **verified**: all checks for the stated scope passed.

Failed outranks unverified, which outranks verified. Warnings carry information and never change the verdict.

The second inventory is the account-bound encrypted JSON export ([ADR 0001](0001-official-cli.md) limits the sources to the unmodified CLI). It is the only public CLI output produced before the decrypt step where the omission happens. Item IDs and, where the item format shows them, passkey counts are compared with the plaintext export. Attachments are accounted for by item ID and attachment ID.

Every result states its scope: the personal items in the local snapshot the CLI exported during this capture. Every result lists the checks that were not possible, among them that completeness against the server is never checked.

## Alternatives

- **Pass/fail.** A single flag either hides items that cannot be checked or fails every capture that contains one.
- **Trusting exit codes and counts.** The reproduced omission returns success with an unchanged item count.
- **Parsing the local CLI profile.** It holds more detail, but its format is internal and unstable, and reading it goes beyond the unmodified CLI.
- **Using the SDK directly.** Internal and unstable, and outside ADR 0001.

## Consequences

- Items whose passkey count the encrypted export does not show, such as blob-format login items, end unverified. If blob items are the norm for new accounts, most real captures will be unverified until a better source exists.
- Verified never means server-complete. An omission in a step that both exports share stays invisible.
- Without an independent reference inventory, a passkey swapped for another valid one and a well-formed but wrong key pass.
- The meaning of the three verdicts becomes a user-facing promise. Changing it later needs a superseding ADR.

The rules in detail are in the design of the change capture-verification.
