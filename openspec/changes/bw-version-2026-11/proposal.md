# Proposal

## Why

Every piece of evidence this project has about `bw` belongs to version 2026.10.1: the dev container pins it, the code notes read its bundle, and the Vaultwarden suite ran against it. `bw` 2026.11.0 was released on 19 November 2026 and 2026.11.1 on 24 November 2026. Users update `bw` themselves, and [ADR 0007](../../../docs/decisions/0007-bw-versions.md) lets any version run without a warning. So a user who updates today runs a combination that nobody has checked.

The release notes of 2026.11.0 touch the places the program relies on. Vault decryption now runs in batches (PM-46036), so the form of the decryption error lines may change, and the client counts those lines by their text. The CLI now exits as soon as a command finishes, with changes to `export.command.ts` (PM-45308). Restricted ("gated") items are handled in new ways in vault lists and exports (PAM-335, PAM-336, PAM-341, PAM-466). Organization cipher fetches can now return decryption failures (PM-46343). Collections get a "shared folder" alias in the CLI (PM-40202). The commercial release zips have new file names. 2026.11.1 only adds Secrets Manager commands.

ADR 0007 also promises a signal at run time when `bw` changes what it exports: "Differences in the export fields are reported at run time by a later change." That change was never made (architecture roadmap, item 5). Today a new field in an export is stored verbatim but never noticed, and the verification cannot judge it.

## What Changes

- **Pin `bw` 2026.11.1** in the dev container (`.devcontainer/package.json` and its lockfile). `vaultwarden.yml` builds that image, so the suite in CI uses the same version.
- **Read the 2026.11.1 bundle against every assumption the client makes about `bw`**, and record the result as new code notes under `docs/research/`. The list includes the decryption error marker, the premium sentence, the status document, the session key form, `BW_CLEANEXIT` and `BW_NOINTERACTION`, the list and export filters including the new handling of restricted items, the absence of syncs in `list`, `export` and `get`, the organization export, the attachment download and the new exit after a command. A changed assumption stops the change for a maintainer decision.
- **Run the unit tests and the Vaultwarden suite against 2026.11.1**, in the dev container and through `vaultwarden.yml`, and add rows to the list of tested `bw` versions.
- **Tighten the tampered-passkey scenario** so that it also requires the `unverified document_decryption_errors` finding for the plaintext export. Every recorded run showed this finding, but no test required it. If a `bw` update changes the text of the lines, the suite then fails instead of counting zero lines without anyone noticing. The intact scenario requires that no export key is reported as unknown, which tests the known keys against a real export.
- **New: report export fields unknown to this project.** The parsers of the plaintext and the encrypted export, personal and per chosen organization, compare the keys they meet with the keys this project knows from the `bw` source and the code notes. Each unknown key path produces a warning with the document, the organization ID where it applies, the path and the number of items that carry it. An unknown item type number produces a warning as well. Warnings never change the verdict.
- Update ADR 0007 in place (it stays proposed), the limitations, the test strategy, the integration notes, the research overview and the roadmap.

No command, flag, exit status or archive entry changes. The manifest format version does not change, because the result keeps its form and only gains new reason codes.

Out of scope: unknown keys in the item lists, a check of the values of known fields beyond the existing rules, Bitwarden Cloud, macOS and Windows with the real `bw` 2026.11.1, and Vaultwarden versions other than 1.37.4.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `capture-verification`: three new requirements. "Export keys unknown to this project are reported" covers the warnings for unknown key paths, "Item types unknown to this project are reported" covers unknown item type numbers, and "Reported key paths hold no content" makes sure the result never echoes a key that is not an identifier and limits the number of warnings. The requirement "No secrets in results" is unchanged and continues to apply.
- `backup-run`: the requirement "Report holds no vault content and no bw text" gains one narrow exception: the key path of a warning about an unknown export key may hold key names of identifier form, and no other text of the export. A new scenario tests it (review of 24 November 2026, findings S1 and C1, maintainer decision).
- `bw-invocation`: the requirement "Failures carry a class and no CLI text" gains the same narrow exception, so that it no longer forbids the key path that the two requirements above allow. Its scenarios are unchanged (second review of 24 November 2026, finding S1).

## Impact

**Product requirements addressed** ([requirements](../../../docs/product/requirements.md)): "Export scope, errors and areas that cannot be verified must stay visible." An export that holds data the verification does not know is such an area.

**Requirements touched without a delta:**

- `backup-run` "Report of the run": the report already prints every warning with its `field` and `count`. "Report holds no vault content and no bw text" is now modified, see above. The review showed that the earlier comparison with `passkey_field_missing` does not hold, because those field names are literals of this project, while an unknown key name is copied from the export.
- `bw-invocation` "Failures carry a class and no CLI text" is now modified, see above. The decryption error marker and the premium sentence are touched without a delta: the code notes confirmed both for 2026.11.1.
- `offline-verification`: it prints only the number of warnings. Older generations still read, because the manifest reader accepts reason codes it does not know.

**Code:**

- `internal/capture`: the known key sets, the collection of unknown key paths and item types during parsing, and the package documentation of the document forms.
- `internal/verify`: three warning reason codes, a field for the item type number, and the conversion into warnings.
- `internal/report`: the item type number is printed as `item_type=<n>`.
- `internal/e2e`: the tampered scenario also requires the decryption error finding, and the intact scenario requires that no export key is unknown.
- `.devcontainer/package.json` and `package-lock.json`.
- No change to `cmd/bw-takeout`, `internal/manifest`, `internal/archive` or the workflows.

**Dependencies:** `@bitwarden/cli` 2026.11.1 in the dev container. Its npm dependency tree changes, so the list of known advisories in the integration notes is updated. No Go dependency changes.

**Documentation to update:**

- `docs/research/bw-cli-code-notes-<date>.md` (new) and a row in `docs/research/research-summary.md`.
- `docs/development/integrations.md`: tested `bw` versions, known advisories, and the sentence on the automated runs.
- `docs/development/testing.md`: spec coverage of the new requirement, the stricter tampered scenario, a record of the runs, and the macOS download instructions with the new asset names.
- `docs/user/limitations.md`: what the warning on unknown fields shows and what it does not.
- `docs/user/security.md`: key names in the report, and the residual risk of design D5.
- `docs/decisions/0007-bw-versions.md`: the bullet on export fields describes the warning that now exists.
- `docs/development/architecture.md`: roadmap item 5, with this follow-up done.

**Existing ADRs:**

- [0001 Official bw CLI](../../../docs/decisions/0001-official-cli.md), accepted: unchanged. The project still runs the unmodified `bw`.
- [0004 Completeness verdict](../../../docs/decisions/0004-completeness-verdict.md), proposed: unchanged. Warnings do not change the verdict, as the ADR already says.
- [0006 Secret boundary](../../../docs/decisions/0006-secret-boundary.md), proposed: updated in place with one narrow exception to "Text of `bw` is never copied into a failure or an output": identifier-shaped key names of an export in the key path of a warning (design D5, review of 24 November 2026, maintainer decision). It stays proposed.
- [0007 Handling of bw versions](../../../docs/decisions/0007-bw-versions.md), proposed: updated in place. The version still never gates a run or produces a message. The promised signal about export fields now exists.
- 0002, 0003, 0005 and 0008 are not touched.

**Open evidence:**

- Whether `bw` 2026.11.1 works against Vaultwarden 1.37.4. Vaultwarden 1.37.3 was incompatible with `bw` 2026.9.0 ([CLI analysis](../../../docs/research/bitwarden-cli-analysis-2026-10-31.md)). An incompatibility stops the change for a maintainer decision.
- Whether the batched decryption still writes one line with the marker `ERROR bitwarden_crypto::` per failure, and still on standard error. Only the code notes and the tampered scenario will show this.
- Whether "gated" or restricted items can reach a personal or organization export on Vaultwarden or Bitwarden Cloud at all. Vaultwarden has no PAM features, so the suite cannot produce such items.
- Which export keys 2026.11.1 adds compared with 2026.10.1. This is read from the release source in task 1.
- Bitwarden Cloud remains untested. All results of this change are Vaultwarden evidence.
