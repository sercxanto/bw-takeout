# Design

## Context

See [proposal.md](proposal.md) for motivation. The repository contains no product code yet. The integration boundary is the unmodified official CLI ([ADR 0001](../../../docs/decisions/0001-official-cli.md)), pinned to 2026.10.1.

Evidence this design builds on, with its level:

- **Reproduced at SDK and export-service level** ([completeness experiments](../../../docs/research/completeness-decision-2026-10-31.md)): tolerant decryption can return an item as successful with all passkeys removed. The account-bound encrypted export still showed both credentials for a legacy item. For a blob item it showed only the outer ID. An equal count with a different credential went undetected.
- **Code level, installed 2026.10.1 bundle, read on 13 November 2026**:
  - The account-bound encrypted export is a JSON document with `encrypted: true`, a key validation field, `folders` and `items`. It is built from the encrypted items and skips partial, deleted and restricted items.
  - The password-protected export is a different document with `passwordProtected: true` and a single encrypted `data` string. It is derived from the plaintext export and inherits its omissions.
  - `bw list items` returns item objects that add `attachments` with `id`, `fileName`, `size`, `sizeName` and `url`, plus revision, creation and deletion dates.
  - Upstream detects a blob item by the string `"format_version"` inside the item's `data` field.
- **Not confirmed by any live run**: all of the above for a real account, and whether the three documents apply the same filters.

## Goals / Non-Goals

**Goals:**

- A verification that is a pure function: capture in, result out, no process, file or network access.
- Rules that fail safe: anything not positively recognized lowers the verdict.
- Tests that run offline in the dev container with synthetic data generated at test time.
- A result type that the later manifest can embed unchanged.

**Non-Goals:**

- Starting `bw`, logging in, syncing, downloading attachments or checking sync freshness.
- Comparing two captures of the same run for stability.
- Writing archives, encryption, a command-line interface or any file output.
- Decrypting anything in the encrypted export.
- Organization vaults beyond counting visible items.

## Decisions

### D1: Verification is a pure function over a capture model

Two internal packages at the module root:

- `internal/capture`: the data model of one capture and the parsers that turn the three CLI documents into it. Later changes add the code that collects a capture from `bw`.
- `internal/verify`: the rules and the result type.

Parsing is separate from judging so that a changed CLI output shape touches only the parsers.

*Alternative considered:* one package that reads documents and judges in one pass. Rejected because rule tests would then depend on JSON shapes that are not yet confirmed live.

### D2: Three verdicts with fixed precedence

Failed means a proven deviation or broken input. Unverified means a required check was not possible. Verified means all checks for the stated scope passed. Failed outranks unverified, which outranks verified. Warnings carry information and never change the verdict. This follows the release rules in the completeness research.

*Alternative considered:* a numeric score or a pass/fail result. Rejected because a single flag would either hide unverifiable items or fail every capture that contains one.

### D3: The account-bound encrypted export is the second inventory

It is the only public CLI output that is produced before the decrypt step, which is where the reproduced omission happens. The local CLI profile and the internal SDK would give more detail, but both are internal, unstable and outside ADR 0001.

*Trade-off:* the encrypted export shares the sync source, the scope filters and parts of the export mapper with the plaintext export. An omission in a shared step hits both and stays invisible.

### D4: Item formats are recognized positively

An item in the encrypted export is treated as legacy only if it matches the known legacy form, and as blob only if it matches the known blob form. Everything else is unknown. Blob login items and unknown items produce an unverified finding. Non-login blob items cannot hold passkeys, so the ID comparison is their complete check.

*Alternative considered:* treat "has a `data` field" as blob. Rejected because the research notes that legacy raw data can also carry such a field.

### D5: Any ID difference between the two exports fails

An ID that is only in the encrypted export matches the reproduced decrypt omission. An ID that is only in the plaintext export means the two documents do not describe the same snapshot. Both directions fail, because the verification cannot tell a harmless timing difference from a loss. The later orchestrator decides about retries.

*Risk accepted:* if the two exports turn out to filter differently in a live run, every capture would fail. The live probe will show this. The rule is then adjusted in a follow-up change with the evidence attached.

### D6: Passkey structure rules

Required and non-empty: `credentialId`, `keyType`, `keyAlgorithm`, `keyCurve`, `keyValue`, `rpId`, `counter`, `discoverable`, `creationDate`. Optional: `userHandle`, `userName`, `rpName`, `userDisplayName`.

For the declared combination `public-key`, `ECDSA`, `P-256`, the key value must decode and parse as a PKCS#8 P-256 private key. Any other declared combination is reported as unverified, not failed, because the verification has no evidence about other forms. The counter must be a non-negative integer and is accepted as a string of digits or as a JSON number. The research observed a string.

Parsing uses the Go standard library. The parsed key is discarded immediately. No signing test is performed.

### D7: Attachments are accounted for by ID pair

The expected set is every pair of item ID and attachment ID that the item list shows on a personal item. Each pair needs exactly one download result, which is either an error or a content hash with a byte count. A missing result, an error or a result for an unexpected pair fails.

An empty attachment is a warning, not a failure. Empty files are legitimate, and the `size` metadata may describe ciphertext, so it is not compared with the downloaded length.

*This corrects the explore session,* which had listed an empty download as a failure.

### D8: Three-way item coverage

The item list is the only source for attachment metadata. A personal item in the list that is missing from the plaintext export fails. An exported item that is missing from the list is unverified, because its attachments cannot be known. Organization items in the list are counted per organization ID, reported as a warning and excluded from every comparison.

The caller supplies one merged item list. Whether archived items need a separate listing is a question for the later capture change.

### D9: Optional reference inventory

A reference inventory holds, for known item IDs, the credential ID, a SHA-256 fingerprint of the decoded key value and the counter of each passkey, and the SHA-256 of each attachment. It never holds key material. When supplied, every difference fails. When absent, the result lists credential identity and attachment content as not checked independently.

The reference exists so that tests can prove both sides of the equal-count swap. The same type serves the later offline read test with a synthetic inventory, which the product requirements ask for.

### D10: Results carry IDs, never content

A finding has a severity, a reason code, optional item and attachment IDs, optional counts and an optional passkey position within the item. Reason codes are lowercase snake case constants, for example `item_missing_from_export`, `passkey_count_mismatch`, `passkey_count_unverifiable`, `attachment_download_failed`. The result serializes to JSON deterministically, sorted by item ID, attachment ID and reason code.

Names, user names, URIs, notes, field values, credential IDs, key values and attachment file names never enter a finding. Error text from a failed download is not copied into the result, because CLI error messages can contain names. The download result carries an error class chosen by the caller instead.

### D11: Synthetic fixtures are built at test time

Test helpers build the three documents in memory. Passkey keys are generated with the standard library on every test run, so no key material is committed. This follows the rule in CONTRIBUTING that forbids keys in fixtures. Encrypted fields in the synthetic encrypted export are opaque placeholder strings in the CLI's encrypted-string notation. The verification never decrypts them.

Every sensitive field in the fixtures carries a recognizable sentinel value, which the redaction test searches for in the serialized result.

*Alternative considered:* committed golden files captured from a real run. Deferred until the live probe has produced them. They would still need keys removed or generated.

### D12: Module path and dependencies

The module is named `bitwarden-backup` until a hosting location is settled. Renaming it later is a mechanical change. The module uses the standard library only. The Go version in `go.mod` matches the dev container.

### D13: A proposed ADR records the verdict model

The meaning of "verified" is an essential security assumption and hard to change once users rely on it, so it meets the ADR criteria in CONTRIBUTING. The new ADR `0004` starts as proposed and covers D2, D3 and the scope statement. Context: the reproduced silent omission and the lack of an authoritative server inventory. Alternatives: pass/fail, trusting exit codes, parsing the local profile, using the SDK. Consequences: captures with blob items are unverified, and verified never means server-complete.

The other decisions in this document are change-specific and stay here.

## Architecture baseline to document

The explore session produced a component model and a roadmap that exist nowhere in the repository yet. This change writes both into `docs/development/architecture.md`, marked as planned.

Four rules:

1. Only one component knows `bw`. Everything else works against an interface and is testable with a fake.
2. Verification is a pure function over a captured data model.
3. Plaintext exists only in process memory and inside the encryption stream, never as a file.
4. Only one component touches the target directory, and it only creates new names.

| Component | Responsibility |
|---|---|
| `bwcli` | Start `bw`, clean the environment, check version and hash, pass the session to child processes only, collect stdout, classify errors |
| `profile` | Own CLI data directory per run, detect leftovers of earlier runs, teardown |
| `capture` | Capture model and parsers (this change). Later: collect the documents and attachments and compare before and after |
| `verify` | Turn a capture into a verdict with reasons per ID (this change) |
| `manifest` | Scope, time window, versions, hashes, verdict, unverifiable areas |
| `archive` | Write and read the container, encryption as a stream |
| `publish` | Create a new generation exclusively, write a partial file, sync, publish without replacing |
| `report` | User output with redaction, exit codes |
| `run` | Order of phases and teardown, also on abort |

Roadmap:

1. `capture-verification` (this change).
2. `archive-generation`: container, encryption, manifest, exclusive publishing and an offline verify command.
3. `bw-process-adapter`: `bwcli` and `profile` against a fake `bw`, plus unauthenticated tests with the real binary.
4. `interactive-backup-run`: login, sync freshness, capture, teardown and user documentation. This completes the first prototype.
5. Later: platform hardening, configuration and multiple accounts, organizations, release, then batch operation and a KDBX derivative.

A live probe against a disposable Vaultwarden instance runs outside this sequence and informs steps 1 and 3.

ADRs expected along the roadmap, beyond this change: archive format and layout, the plaintext and secret boundary, and how `bw` versions are qualified. ADRs 0002 and 0003 stay proposed until their open points are tested.

## Risks / Trade-offs

- [Input shapes are derived from source code, not from a live capture] → Positive recognition with unverified as the fallback (D4). The fixture sources are recorded with links. The live probe supplies real shapes later.
- [Blob items may be the norm for new accounts, so most captures end unverified] → Reported honestly rather than hidden. The finding count shows how much of the vault is affected. This is a product-level question for after the live probe.
- [The two exports may filter differently and cause false failures] → Accepted for now (D5). The reason codes make the direction of the difference visible.
- [A well-formed but wrong key, or a swapped credential, passes without a reference] → Stated as a standing limit in every result and in the user limitations.
- [The attachment inventory has one source, the decrypted item list] → An item that fails decryption also disappears from the list, but then fails the ID comparison (D5). An attachment missing from an otherwise intact list entry stays undetected. Documented as a limit.
- [The Go runtime may keep parsed key bytes in memory] → Keys are parsed only to check their form and dropped at once. Memory hygiene belongs to the later ADR on the plaintext boundary.
- [Strict passkey rules could fail real passkeys with unusual forms] → Only the positively known P-256 form is judged. Other forms are unverified.

## Open Questions

- Should an empty attachment stay a warning once live data shows how the `size` metadata behaves?
- Does the reference inventory need a file format? This change only defines it as an in-memory type.
- Final module path, once a hosting location exists.
