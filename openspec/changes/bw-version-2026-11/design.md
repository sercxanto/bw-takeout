# Design

## Context

See [proposal.md](proposal.md) for the motivation and scope. This is the state the design builds on:

- **The pin.** `.devcontainer/package.json` pins `@bitwarden/cli` 2026.10.1 and `openspec`. The Dockerfile installs both with `npm ci` and links `bw` to `/usr/local/bin/bw`. `vaultwarden.yml` builds this image and runs `internal/e2e` inside it, so the pin decides the `bw` of the dev container and of the CI suite. Test fakes answer `--version` with `2026.10.1`. That string is fixture data, and no product code compares it with anything ([ADR 0007](../../../docs/decisions/0007-bw-versions.md)).
- **What the client relies on.** The code notes of [15](../../../docs/research/bw-cli-code-notes-2026-11-15.md), [16](../../../docs/research/bw-cli-code-notes-2026-11-16.md) and [17 November 2026](../../../docs/research/bw-cli-code-notes-2026-11-17.md), the [organization spike](../../../docs/research/organization-export-spike-2026-11-21.md) and the package documentation of `internal/capture` and `internal/bwcli` record each fact about `bw` that the code depends on. Two of them are literal texts: the decryption error marker `ERROR bitwarden_crypto::` and the premium sentence in `internal/bwcli/stream.go`. The marker decides whether a document is judged unverified.
- **Release notes of 2026.11.0** (read on 24 November 2026 from the GitHub release `cli-v2026.11.0` and the linked pull requests). These are the entries that could touch the assumptions above:
  - PM-46036 / #24241 and #24305: vault decryption in batches in `cipher.service.ts`.
  - PM-45308 / #23953: the CLI exits once a command has finished and flushes standard output and standard error first. `export.command.ts` and `bw.ts` changed.
  - PAM-335, PAM-336, PAM-341 and PAM-466: restricted ("gated") items in vault lists and exports. #24253 states that gated ("partial") items were already dropped from organization exports by #22171, and adds a count and a warning in the web export page only.
  - PM-46343 / #24447: new organization cipher fetch methods that return decryption failures. The existing methods keep their behavior.
  - PM-40202 / #23897: collection commands get a "shared folder" alias.
  - PM-28207 / #23902 and PM-28195 / #23921: the feature flags `pm-28192-cipher-attachment-ops-to-sdk` and `pm-27632-cipher-crud-operations-to-sdk` are removed. The server provides the flags. If Vaultwarden 1.37.4 did not set them, `bw` 2026.10.1 downloaded attachments on the old path there, and 2026.11.x may use the SDK path for the first time.
  - The commercial single-file zips are renamed to `bw-bit-<os>[-arm64]-<version>.zip`.
  - 2026.11.1 only adds Secrets Manager commands (`bw sm …`).
- **The parsers.** `internal/capture` decodes each export into `map[string]any` and reads the keys it needs. It ignores every other key. `typeObjectKeys` lists the eight known item types. In the encrypted export an item with an unknown type is already `FormatUnknown` and leads to `unverified item_format_unrecognized`. The plaintext export does not look at the type.
- **Findings.** `verify.Finding` already has `Document`, `OrganizationID`, `Field` and `Count`, and `report.Finding` prints each field that is set. The manifest stores the result as JSON and `manifest.Parse` ignores keys it does not know.
- **The tampered scenario** of `internal/e2e` requires exit status 1, `failed passkey_count_mismatch` as the only failed finding, and no generation. Every recorded run also showed `unverified document_decryption_errors count=3` for the plaintext export, the item list and the archived item list ([testing](../../../docs/development/testing.md)), but no test requires it.

## Goals / Non-Goals

**Goals:**

- The dev container and the CI suite run the `bw` that users get today, and the project's evidence says which assumptions still hold for it.
- A silent break of the decryption error count turns into a failing test.
- When `bw` exports something the project has not seen, the user sees a precise warning without any vault content, and the verdict stays honest.

**Non-Goals:**

- Adapting the client to a changed `bw` behavior. Such a change goes to a maintainer decision (D2).
- A check on the values of known keys, beyond the existing passkey and format rules.
- Unknown keys in the item lists (D7).
- A gate or a message based on the `bw` version, which ADR 0007 rejects.

## Decisions

### D1: Pin 2026.11.1

The dev container pins `@bitwarden/cli` 2026.11.1, and the lockfile is regenerated with `npm install` inside the container. Test fakes keep `2026.10.1` as their version string. It is fixture data, and changing it would touch about twenty tests for nothing.

*Alternative considered:* 2026.11.0. Rejected, because npm hands out 2026.11.1 to every new install, and its only difference is the Secrets Manager commands, which the client never calls. The code notes of D2 confirm that no other file of the vault path changed between the two.

### D2: One register of assumptions, checked against the release source

The new code notes `docs/research/bw-cli-code-notes-<date>.md` hold a table with one row per assumption. Each row gives the source of the assumption (code notes, spike, package documentation), the code that depends on it, where in the 2026.11.1 source or bundle it was checked, and the result: unchanged, changed or not determinable. The rows are at least:

| Assumption | Depends on it |
|---|---|
| Any non-empty `BW_CLEANEXIT` turns a failure into exit code 0, and `BW_NOINTERACTION=true` stops the prompts | `bwcli` environment |
| Prompts go to standard error. `login --raw` prints only the session key, in its known form | `bwcli.Login` |
| The status document and its states | `bwcli/status.go` |
| The decryption error line starts with `ERROR bitwarden_crypto::` and goes to standard error, one line per failure | `bwcli/stream.go`, decryption findings |
| The premium sentence of the attachment download | `bwcli/stream.go` |
| `list items` and `--archived`: filters, and the premium fallback that drops `--archived` | `collect`, `archive_filter_not_applied` |
| `list`, `export` and `get attachment` start no sync (search for `fullSync` calls) | the freshness rule of `collect` |
| Export forms, the blob marker `"format_version"`, and the export filters (deleted, partial, restricted, organization), including the new handling of gated items | `capture`, `verify` |
| The organization export reads from the server, exports the whole organization and checks no permission in the client | `organization-export` |
| `get attachment` writes the decrypted bytes and turns a status other than 200 into an error | `collect` |
| The path that `get attachment` takes (old path or SDK) before and after the removal of the flag `pm-28192-cipher-attachment-ops-to-sdk`, and whether the premium sentence, the failure texts and the decryption error lines of a download stay the same on it | `bwcli` download classes, `attachment_decryption_errors` |
| The new exit after a command flushes standard output before it exits, so a large document is never cut off | `bwcli` output reading |
| The key sets of the export models (D4) | `capture` |

The source is the `bitwarden/clients` repository at the release commits of `cli-v2026.10.1` and `cli-v2026.11.1`, compared with `git diff` on the paths the rows name. The installed bundle of 2026.11.1 is used to confirm literal texts. The notes give the hash of `build/bw.js`.

If a row reads "changed" in a way that affects the client, the task stops and asks the maintainer. The client is not adapted inside this change unless the maintainer decides so, because an adaptation changes the evidence base that the specs rest on.

*Alternative considered:* rely on the Vaultwarden suite alone. Rejected, because the suite cannot produce blob items, a lapsed premium, gated items or a decryption error on an attachment, and a changed marker would only show as a missing finding, which no test required until D3.

### D3: The tampered scenario requires the decryption error finding

`TestTamperedPasskeyFails` also requires an `unverified document_decryption_errors` finding for the plaintext export with a count of at least 1. It does not require exactly 3, because the count depends on how `bw` batches the decryption. It does not require the findings for the two item lists. They were present in every run, but tying the test to them adds nothing to the purpose of the check, which is to notice a marker that no longer matches.

*Alternative considered:* a unit test with the 2026.11.1 line as a literal. Rejected as the only measure, because the text would have to be read off by hand and becomes outdated with the next version. The code notes record the literal, and the existing `bwcli` tests keep covering the counting.

### D4: Known keys come from the export models of both versions

The known key sets live in `internal/capture`, next to `typeObjectKeys`, with one set per place:

- the document level of the personal plaintext export, of the personal encrypted export, of the organization plaintext export and of the organization encrypted export,
- an element of `folders` and an element of `collections`,
- an item. It is one set for both forms, including `key` and `data`, so that legacy and blob items pass,
- each type-specific object (`login`, `secureNote`, `card`, `identity`, `sshKey`, `bankAccount`, `driversLicense`, `passport`),
- an element of `login.uris`, of `login.fido2Credentials`, of `fields` and of `passwordHistory`.

Each set is the union of the keys that the export model classes write in 2026.10.1 and in 2026.11.1 (`libs/common/src/models/export/*.export.ts` and the document types in `libs/tools/export-vault-core`). Using the union means that neither version produces a warning. The code notes of D2 list the keys that 2026.11.1 added, if any. The package documentation of `capture` is updated to match.

The parser compares keys while it already walks the document. It never descends into the value of an unknown key, and it does not descend into a known key whose value has an unexpected type, because the existing rules already judge that case. For each place it records the unknown key paths and the number of top-level elements (items, folders, collections) that carry each path. A document-level path has the count 1.

A path is written from the document root, with `[]` for an array element: `items[].login.fido2Credentials[].newKey`, `folders[].newKey`, `newKey`.

The plaintext parser also counts items whose integer `type` is not a key of `typeObjectKeys`, per type number. The encrypted export keeps its existing rule (unknown type means unrecognized format).

*Alternative considered:* compare the keys with those of a stored sample document. Rejected, because a sample covers only the item types and optional keys it happens to hold, and the model classes are the authority.

*Alternative considered:* one key set per place and form, so that a `key` in the plaintext export would be reported. Rejected for now. It doubles the sets for a case that the code reading says cannot happen, and D2 would notice a change in the model.

### D5: Key paths are codes, never content

A key enters a path only if it matches `^[A-Za-z][A-Za-z0-9]{0,63}$`. Every key in the 2026.10.1 export models has this form. Any other unknown key is recorded as the placeholder `<other>` at its place (for example `items[].login.<other>`), and its text is never copied out of the decoded map. The filter is applied in `capture`, so no string that fails it reaches `verify`, the manifest or the report.

The report prints the path through `clean` like every other field. A path that passed the filter is part of the schema of `bw`, not of the vault. So printing it is in line with the `backup-run` rule that the report holds no vault content and no text of `bw`, in the same way that `passkey_field_missing` prints a passkey key name today.

Residual risk: if a future `bw` exports an object whose keys are user data, and an identifier-shaped key in it happens to be content, that key could appear in the report. This can only happen at a known place, because unknown values are never walked, and the known places are objects with a fixed schema in the source. The risk is recorded in the security page and the limitations.

*Alternative considered:* report only the number of unknown keys, without names. Rejected, because the warning then tells neither user nor developer what changed, and that is what ADR 0007 asks for.

The comparison with `passkey_field_missing` above does not hold (review of 24 November 2026, findings S1 and C1): those field names are literals of this project, while an unknown key name is copied from a document that `bw` wrote. The report therefore contradicted the wording of the `backup-run` requirement "Report holds no vault content and no bw text" and of [ADR 0006](../../../docs/decisions/0006-secret-boundary.md), "Text of `bw` is never copied into a failure or an output". Maintainer decision of the same day: keep the behavior and make a narrow exception in both. The change adds a MODIFIED delta for `backup-run` with the scenario "Key path of an unknown export key" (`run.TestKeyPathOfAnUnknownExportKey`), and ADR 0006 is updated in place and stays proposed. The alternatives were to print only the place with a placeholder, which loses the signal ADR 0007 asks for, and to keep the wording, which would leave the archived spec in conflict with the code.

### D6: Warnings, capped per document

`verify` turns the records into warnings:

- `export_key_unknown`: `document`, `organization_id` where it applies, `field` = the path, and `count`.
- `export_key_unknown_omitted`: `document`, `organization_id`, and `count` = the number of paths left out. It is emitted when a document has more than 32 distinct paths. The 32 paths reported are the first in byte order.
- `export_item_type_unknown`: `document`, `organization_id`, `item_type` = the type number, and `count`.

`verify.Finding` gains the field `ItemType *int` (`json:"item_type,omitempty"`), and `report.Finding` prints it as `item_type=<n>`. The result sorts the warnings like every other finding, by organization ID, reason code, document, then path or type number, so that the report and the manifest are deterministic (review of 24 November 2026, finding C2: the design first said capture order, which the common sort of the result does not keep). All three reasons are added to `verify.Reasons()`.

None of them changes the verdict. The documents are stored as `bw` wrote them, so an unknown key loses nothing from the archive, and a verdict of `unverified` after each `bw` update would teach users to ignore it. That is the argument ADR 0007 makes against a version warning. An item whose passkeys become invisible in only one of the exports still lowers the verdict through the existing format rules. If `bw` moved or renamed `login.fido2Credentials` in both export models, both sides would count zero passkeys, the verdict would stay verified, and only the two `export_key_unknown` warnings would show the new key (review of 24 November 2026, finding S2, reproduced with a test). Maintainer decision of the same day: document the case in the limitations and keep the warnings as warnings, because a rule that lowers the verdict for any unknown key of a login would lower every login after a harmless addition, such as `httpAuthenticationEnabled`, which the 2026.11.1 domain model already holds. The code notes of each new `bw` version check the passkey place.

*Alternative considered:* severity unverified. Rejected for the reason above.

*Alternative considered:* the type number in `found` or in `field`. Rejected, because `found` means a count in every other finding, and `field` means a key.

### D7: The item lists are not checked

The item lists are `CipherResponse` objects with more keys than the exports, several of them for the CLI's own presentation. They change more often and are not what a user restores from. Checking them would add noise without adding a signal for the archive. They remain a possible later step.

### D8: Documentation follows the pin

- `docs/development/integrations.md`: rows for 2026.11.1 in the tested versions table (Linux arm64 dev container, Linux amd64 through `vaultwarden.yml`, macOS and Windows not run), the sentence on the automated runs, and the known npm advisories after `npm audit` in the container.
- `docs/development/testing.md`: the macOS download instructions of the manual exercises use the pin and the new asset name `bw-bit-macos-arm64-2026.11.1.zip`, with the digest that GitHub lists for it. The records of earlier runs stay unchanged.
- ADR 0007 is updated in place: the bullet "Differences in the export fields are reported at run time by a later change" now describes the warnings of D6. The ADR stays proposed.

No new ADR. The warning realizes a decision that ADR 0007 already records, it is easy to reverse, and it does not move the integration boundary. It does narrow the secret boundary of ADR 0006 by one exception for identifier-shaped key names, which is recorded in that ADR in place (review of 24 November 2026, see D5).

## Risks / Trade-offs

- [`bw` 2026.11.1 does not work against Vaultwarden 1.37.4] → The task stops and asks the maintainer. The options then are to wait for a Vaultwarden release, to pin 2026.11.1 without e2e evidence, or to stay on 2026.10.1. None of them is chosen in advance.
- [The decryption marker changed] → D2 finds it in the source, and D3 fails the suite. The change stops for a decision instead of silently counting zero lines.
- [The model classes are read wrongly and a known key is missing from a set] → Every intact Vaultwarden run would show a warning. The intact scenario therefore also requires that the result has no `export_key_unknown` warning, which tests the sets against a real 2026.11.1 export.
- [A real export holds a key that the models do not show, for example from a server extension of Vaultwarden] → It is reported as a warning. This is what the check is for. If it shows up in the intact scenario, the task records it and asks before adding it to a set.
- [Warnings about unknown keys become routine after `bw` updates and users stop reading them] → The warnings name the path, and each one disappears once a release adds the key to the known set. ADR 0007 already accepts that releases follow `bw` versions for this signal.
- [Gated items are dropped from every document, including the item lists] → The verification cannot see them. This is the existing limit "items the CLI leaves out of every document". The code notes say whether 2026.11.1 extends that filter to personal exports, and the limitations name gated items explicitly if it does.

## Migration Plan

Nothing to migrate. Generations written before this change have no warnings of the new kinds and still read. Older binaries read new manifests, because `manifest.Parse` ignores the unknown key `item_type` and reason codes are not checked against a list when reading. Rolling back the pin is a one-line change of `package.json` and its lockfile.

## Open Questions

- Which exact keys 2026.11.1 adds to the export models. Task 1 answers this, and the answer only fills the sets of D4.

## Review of 24 November 2026

Code review and security review in fresh contexts. Every finding was checked against the code, the spec delta and this design. None was rejected.

- **S1 and C1** (key names of `bw` in the report against `backup-run` and ADR 0006): confirmed, maintainer decision, see D5.
- **S2** (passkeys renamed in both exports stay verified): confirmed with a test, maintainer decision to document, see D6.
- **C2** (sort order of the warnings): confirmed, design and code comment corrected, see D6.
- **C3** (plaintext item types outside 0 to 65536 were not counted, although the spec delta names every integer type): confirmed and fixed. The count now accepts every integer that fits into an int, with `TestUnknownItemTypeOfAnyInteger`. The encrypted export keeps its rule.
- **C4** (two comments said the parsers read neither folders nor collections): confirmed, corrected in `internal/capture`.
- **C5** (the run record said "Both runs" for three runs): confirmed, corrected in the test strategy.
- **C6** (two package comments named only 2026.10.1 as evidence): confirmed, `internal/collect` and `internal/bwcli` now name the code notes of 24 November 2026 as well.

Second round, with fresh reviewers. Every fix R1 to R7 was confirmed. None of the new findings was rejected.

- **S1, second round** (the `bw-invocation` requirement "Failures carry a class and no CLI text" still forbade any text of `bw` in a result or an output): confirmed. A MODIFIED delta gives it the same narrow exception as `backup-run` and ADR 0006, which applies the maintainer decision of D5 to the third place that states the rule. Its scenarios are unchanged.
- **C1, second round** (the fixture run "unknown export keys and item types" stayed below the cap, because six of its generated keys were no identifiers and became one `<other>` path): confirmed. The keys are numbered now, and `TestUnknownExportKeysFixtureCoversEveryReason` checks that the run holds all three reasons. It failed before the fix.
- **C2, second round** (the usage page did not say that a warning can name a key path): confirmed, a sentence with a link to the security page was added.

