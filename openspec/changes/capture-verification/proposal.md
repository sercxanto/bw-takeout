# Proposal

## Why

The research of 31 October 2026 showed that the official CLI can report success while an item or its passkeys are missing from the export, and that exit codes and matching counts do not prove completeness. Before any backup is written, the project needs a rule set that turns captured CLI output into an honest verdict. This rule set is the product's central risk and can be built and tested first, because it needs no login, no `bw` process and no archive.

## What Changes

- Add a verification core that takes one capture of a personal vault and returns a verdict of failed, unverified or verified, with a reason per affected ID. A capture consists of the plaintext JSON export, the account-bound encrypted JSON export, the item list and the per-ID attachment download results.
- Compare item IDs and per-item passkey counts between the encrypted and the plaintext export, check the structure of every exported passkey, and account for every attachment by item ID and attachment ID.
- Report item formats that cannot be checked independently as unverified instead of treating them as "no passkeys".
- Accept an optional reference inventory so tests can detect content swaps that no comparison of CLI outputs can detect.
- Report organization items as visible but excluded, with counts.
- Keep key material, passwords, names and file names out of all results.
- Add the first Go module to the repository. This change adds no command-line interface, no `bw` invocation, no encryption and no file output.
- Record the verdict model as a proposed ADR and document the component model and change roadmap that came out of the explore session.

## Capabilities

### New Capabilities

- `capture-verification`: Decides whether a captured personal vault inventory is failed, unverified or verified for a stated scope, and explains the decision per item, passkey and attachment without exposing secrets.

### Modified Capabilities

None. No specs exist yet.

## Impact

**Product requirements addressed** ([requirements](../../../docs/product/requirements.md)):

- Passkeys including key material and attachments belong in the archive and must be attributable. This change checks both by ID.
- Export scope, errors and areas that cannot be verified must stay visible, and no absolute completeness guarantee is given. The verdict model and scope statement implement this.

**Code:** new Go module at the repository root with internal packages for the capture model and the verification rules. Standard library only.

**Documentation to update:**

- `docs/development/architecture.md`: component model and change roadmap.
- `docs/development/testing.md`: how to run the tests, evidence level, what was not run.
- `docs/development/getting-started.md`: test command.
- `docs/user/limitations.md`: what the verdict can and cannot detect.
- `docs/decisions/`: new proposed ADR for the verdict model and scope claim, plus the index.

**Existing ADRs:**

- [0001 Official bw CLI](../../../docs/decisions/0001-official-cli.md), accepted: constrains this change to data the unmodified CLI emits. The local CLI profile and the internal SDK are not used as sources.
- [0002 Go](../../../docs/decisions/0002-go.md), proposed: this change adds the first Go code but gives no evidence on interactive process handling or Windows behavior, so it does not move the ADR to accepted.
- [0003 age](../../../docs/decisions/0003-age.md), proposed: not touched.

**Open evidence:**

- The silent passkey omission was reproduced at SDK and export-service level only, not through an authenticated CLI process.
- The shapes of the three input documents come from the pinned upstream source and the installed 2026.10.1 bundle. No live capture exists yet.
- Whether a fresh account produces legacy or blob items is unknown. If blob items are the norm, most real captures will end as unverified.
- Whether the plaintext export, the encrypted export and the item list apply the same filters (archived items, restricted item types) is unconfirmed.
- A live probe against a disposable Vaultwarden instance is planned separately and may lead to adjusted fixtures and rules.
