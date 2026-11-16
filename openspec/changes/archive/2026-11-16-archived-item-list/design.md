# Design

## Context

See [proposal.md](proposal.md) for the motivation and the split of `interactive-backup-run`.

The current state:

- `capture.Input` holds one `ItemList`, `capture.Capture` one parsed `List`, and `verify` builds one index of listed items from it.
- `archive.Documents` holds three documents. The writer stores them after the recovery instructions. The reader accepts the three fixed names and requires them in the manifest.
- `bwcli.Client` already has `ListItems` and `ListArchivedItems`. Nothing calls them outside tests.
- No command writes a generation. Archives exist only in tests, built through `archivetest`.

Evidence for the behavior of `bw` 2026.10.1:

| Statement | Source | Level |
|---|---|---|
| The default list leaves archived items out, both exports keep them, `list items --archived` returns only archived items | [Live probe](../../../docs/research/vaultwarden-live-probe-2026-11-14.md), added case "archived item" | Observed on Vaultwarden 1.37.4, one login without attachments |
| For an account that may not archive, `--archived` is dropped before filtering: `options.archived = userCanArchive && options.archived` | Bundle `build/bw.js` of the npm package, lines 58258 to 58260, filter at 58454 to 58455, read on 16 November 2026 | Code reading |
| Both personal exports filter `deletedDate == null`, and the list filters by `options.trash === c.isDeleted` with `trash` false by default | Same bundle, lines 59248 and 59282 for the exports, 58259 and 58454 for the list | Code reading |

The bundle is the one identified in the package documentation of `internal/capture` by its SHA-256.

## Goals / Non-Goals

**Goals:**

- A vault with archived items can reach the verdict verified, with the attachments of archived items accounted for.
- Both lists stay available in the archive exactly as `bw` wrote them.
- An account for which `bw` ignores the archive filter gets no worse verdict than today, and the situation is visible in the result.

**Non-Goals:**

- Calling `bw`. The two documents are inputs, as the three existing ones are.
- Reading `archivedDate` from the exports, or checking that an item is in the list its archive state calls for.
- Capturing the trash.
- Reading archives of the three-document layout.

## Decisions

### D1. Two documents in the capture, not one merged list

`capture.Input` gets `ArchivedItemList []byte` next to `ItemList`, and `capture.Capture` gets `ArchivedList *ItemList` next to `List`. Both are parsed by the existing `ParseItemList`. A new document name `archived_item_list` joins `item_list`, so that a document error or a duplicate names the list it came from.

An absent archived list is a zero-length input. The parser rejects it as it rejects any other unparsable document, so the verdict is failed. An empty list is the two bytes `[]` and is valid.

*Alternative: merge both outputs into one JSON array before parsing.* Rejected. The archive stores documents byte for byte as captured (spec `backup-archive`), and a merged array is a document `bw` never wrote. It would also hide in which list a duplicate or a malformed element occurred.

*Alternative: make the archived list optional.* Rejected. A capture without it cannot be told apart from one where the collection forgot the call, and that is the defect this change removes.

### D2. The verification works on a combined listing

`verify` builds its index of listed items from `List` followed by `ArchivedList`. Every existing rule reads this index: attachment accounting, attachment inventory coverage, organization items. The rules themselves do not change.

- Duplicates within one list stay a failed finding with the reason `duplicate_item_id` or `duplicate_attachment_id`, with the document set to that list.
- An ID in both lists is not a duplicate. It enters the index once, from the default list.
- The warning `organization_items_excluded` counts an organization item of both lists once.
- A finding that refers to the combined listing names no document, because it refers to two: `exported_item_not_listed` (the item is in neither list) and `organization_items_excluded` (the count spans both lists). Until this change both named `item_list`. A finding about one entry names the list the entry came from, for example `item_organization_mismatch` (maintainer decision, 16 November 2026).
- When either list was rejected, the checks that need the listing are skipped as they are today for a rejected item list. The document finding already makes the verdict failed.

*Alternative: run every rule once per list.* Rejected. An archived item would be reported as "exported item not listed" by the pass over the default list, and each such finding would then need an exception.

### D3. An item in both lists must agree

For an item ID in both lists, the two entries are compared by organization ID and by the set of attachment IDs. A difference is a failed finding with the new reason `listed_item_differs_between_lists` and the item ID. The finding carries no document, because it refers to two.

The comparison is by set. The order of attachments within an entry carries no meaning, and duplicates within an entry are already reported per list.

Agreeing entries produce no finding per item. Whether the overlap as such is reported is D4.

*Alternative: fail on every ID in both lists.* Rejected. For an account that may not archive, `bw` returns the default list for both calls (see Context). Every such account would fail although no data is missing.

*Alternative: ignore the second entry.* Rejected. Two documents of one snapshot that name different attachments for one item show that one of them is wrong, and the attachment inventory has only this one source (standing limit `attachment_inventory_single_source`).

### D4. Overlap is the sign of an ignored archive filter

When at least one item ID is in both lists, the result carries one warning with the new reason `archive_filter_not_applied` and `count` set to the number of shared item IDs. It names no item. It does not change the verdict.

A filter that applies gives disjoint lists by construction, since an item is either archived or not. Overlap is therefore sufficient to recognize the ignored filter and needs no further field. With two empty lists nothing can be recognized, and nothing is lost.

*Alternative: read `archivedDate` from the plaintext export and compare it with the list an item is in.* Rejected for this change. It needs a new field in the model and rules for each mismatch, and it would rest on an export field whose exact form was observed once. Overlap detects the one case the code reading found.

*Alternative: lower the verdict to unverified.* Rejected. For such an account the default list is complete for every item that is not archived. Archived items it may still hold are in neither list and already produce `exported_item_not_listed` with the verdict unverified. The warning adds the explanation and takes nothing away.

### D5. A fourth document, format version unchanged

`archive.Documents` gets `ArchivedItemList`. The entry `documents/items.archived.json` follows `documents/items.json`. The reader accepts the name and requires it, in the manifest and in the stream. The writer refuses a zero-length document for each of the four, before anything is written to the destination.

The layout becomes:

```text
RECOVERY.txt
documents/export.json
documents/export.encrypted.json
documents/items.json
documents/items.archived.json
attachments/<item-id>/<attachment-id>
manifest.json
```

The format version stays 1. No command writes a generation yet, ADR 0005 is proposed, and the only archives are built inside tests. A second version would oblige the reader to keep the three-document layout readable for archives that do not exist.

*Alternative: version 2, with version 1 still readable.* Rejected for the reason above. From the first release on, every layout change needs a new version. This is added to ADR 0005 as a consequence, so that the exception is not repeated.

*Alternative: `documents/items-archived.json`.* The name with a dot follows `export.encrypted.json`, which marks a variant of a document the same way.

### D6. Recovery instructions

`RECOVERY.txt` gets a paragraph for `documents/items.archived.json` and a changed paragraph for `documents/items.json`: the two lists together name the attachments, and an item can appear in both. The existing test that the text names every top-level entry is extended to every document name.

### D7. Fixtures

The intact fixture of `capturetest` gets a fourth item: an archived login with one attachment. It is in both exports with an archive date and only in the archived list. The reference inventory gets its attachment. This makes every existing test run with two non-empty lists.

New variants, each breaking the fixture in one place:

| Variant | Expected |
|---|---|
| Archive filter ignored: archived list equals the default list, the archived item is in neither | unverified through `exported_item_not_listed`, warning `archive_filter_not_applied` |
| Archive filter ignored, no archived item in the vault | verified, warning `archive_filter_not_applied` |
| Item in both lists with different attachment IDs | failed, `listed_item_differs_between_lists` |
| Item in both lists, personal in one and organization item in the other | failed, `listed_item_differs_between_lists` |
| Duplicate ID within the archived list | failed, `duplicate_item_id` with the archived list as document |
| Archived list not valid JSON | failed, document finding for the archived list |
| Archived list absent | failed, document finding for the archived list |
| Download missing for the attachment of the archived item | failed |

The variant "exported item not listed" keeps its meaning and now removes the item from both lists.

### ADR criteria

The archive layout is a lasting choice and is covered by the proposed ADR 0005, which is updated in place. The change sets no new integration boundary and no security assumption, so no new ADR is written. The reason codes are part of the result format that ADR 0004 covers. ADR 0004 is checked for a statement about a single item list and updated if it has one.

## Risks / Trade-offs

- [The ignored filter is known from code reading only] → The rule is built so that a wrong reading costs nothing: overlap with agreeing entries never lowers the verdict. The limitation page says that the case was not observed. An account without the right to archive is not available on Vaultwarden, so a live check needs Bitwarden Cloud and stays open.
- [An archived item with an attachment was never observed live] → The archived list is parsed by the same parser as the default list, which rejects what it does not recognize. A different element form ends as failed, not as a silent pass. The next live run with change C should include this case.
- [A future `bw` could return archived items in the default list as well] → This looks like an ignored filter and produces the warning, with a correct verdict as long as the entries agree.
- [The attachment inventory still has one source per item] → Unchanged. The standing limit stays in every result.
- [Format version 1 changes its meaning] → Accepted once, before any release, and recorded in ADR 0005 together with the rule for later changes.
- [The trash is not captured] → Recorded in the limitations. A user who relies on the trash has no backup of it.

## Migration Plan

None for users, because no generation exists. Developers: archives produced by test code before this change are refused by the reader. No such archive is stored in the repository.
