# capture-verification Specification

## Purpose

Decides whether a captured personal vault inventory may be called complete for a stated scope. It compares the documents the official CLI emitted for one capture and explains every deviation per item, passkey and attachment without exposing secrets.

## Requirements

### Requirement: Three-state verdict
The verification SHALL return exactly one verdict for a capture: failed, unverified or verified. Failed SHALL take precedence over unverified, and unverified over verified. Warnings SHALL NOT change the verdict.

#### Scenario: Intact capture
- **WHEN** a capture with two passkeys and two attachments passes every check
- **THEN** the verdict is verified

#### Scenario: Failure outranks unverifiable items
- **WHEN** a capture contains one item that cannot be checked independently and one item with a proven deviation
- **THEN** the verdict is failed
- **AND** both findings are listed

### Requirement: Scope statement and standing limits
Every result SHALL state the scope its verdict refers to, which is the personal items present in the captured local snapshot. A result SHALL NOT claim that all items on the server are covered. Every result SHALL list the checks that were not possible for this capture.

#### Scenario: Verified result without reference inventory
- **WHEN** a capture is verified and no reference inventory was supplied
- **THEN** the result names the scope
- **AND** the result states that credential identity and attachment content were not checked independently

### Requirement: Document form validation
The verification SHALL reject a capture whose documents do not have the expected form: a plaintext export that is not marked as unencrypted, an encrypted export that is not account-bound, or any document that cannot be parsed.

#### Scenario: Password-protected export supplied as inventory
- **WHEN** the encrypted export is a password-protected export instead of an account-bound one
- **THEN** the verdict is failed
- **AND** the finding names the document and the reason

#### Scenario: Unparsable document
- **WHEN** the item list is not valid JSON
- **THEN** the verdict is failed

### Requirement: Item identity comparison
The verification SHALL compare the item IDs of the encrypted export with those of the plaintext export. Any ID present in only one of the two SHALL produce a failed finding that names the ID and the document it is missing from. A duplicate ID within one document SHALL produce a failed finding.

#### Scenario: Item missing from plaintext export
- **WHEN** an item ID appears in the encrypted export but not in the plaintext export
- **THEN** the verdict is failed
- **AND** the finding names the item ID

#### Scenario: Duplicate item ID
- **WHEN** the plaintext export contains the same item ID twice
- **THEN** the verdict is failed

### Requirement: Passkey count comparison
For every item in a format whose passkey list is visible in the encrypted export, the verification SHALL compare the number of passkeys in the encrypted export with the number in the plaintext export. A difference SHALL produce a failed finding with both counts. A legacy login item without a passkey list in the encrypted export SHALL count as zero passkeys.

#### Scenario: Legacy login without passkey list
- **WHEN** the encrypted export shows a legacy login item without a passkey list and the plaintext export shows an empty passkey list for it
- **THEN** no finding is reported for this item

#### Scenario: One passkey missing
- **WHEN** the encrypted export shows two passkeys for an item and the plaintext export shows one
- **THEN** the verdict is failed
- **AND** the finding states expected two, exported one

#### Scenario: All passkeys dropped
- **WHEN** the encrypted export shows two passkeys for an item and the plaintext export shows none
- **THEN** the verdict is failed

### Requirement: Unverifiable item formats
An item whose passkey list is not visible in the encrypted export SHALL be reported as unverified when it can hold passkeys. The verification SHALL NOT report such an item as having zero passkeys. An item whose format is not recognized SHALL be reported as unverified.

#### Scenario: Blob-format login item
- **WHEN** the encrypted export contains a login item in blob format
- **THEN** the verdict is unverified
- **AND** the finding states that the passkey count of this item cannot be checked

#### Scenario: Unrecognized format
- **WHEN** an item in the encrypted export matches neither the known legacy form nor the known blob form
- **THEN** the verdict is unverified

#### Scenario: Legacy content with unmarked sealed data
- **WHEN** an item in the encrypted export carries legacy content together with sealed data that lacks the blob marker
- **THEN** the verdict is unverified
- **AND** the finding states that the format of this item is not recognized

### Requirement: Passkey structure validation
The verification SHALL check every passkey in the plaintext export for the required fields, a private key that parses for its declared algorithm and curve, and a counter that is a non-negative integer. A violation SHALL produce a failed finding. A declared key form the verification does not know SHALL produce an unverified finding.

#### Scenario: Two valid passkeys with different keys and counters
- **WHEN** an item holds two passkeys with different valid keys and the counters 0 and 6
- **THEN** no finding is reported for these passkeys

#### Scenario: Empty key
- **WHEN** a passkey in the plaintext export has an empty private key
- **THEN** the verdict is failed

#### Scenario: Key does not parse
- **WHEN** a passkey declares a P-256 key but its private key cannot be parsed
- **THEN** the verdict is failed

#### Scenario: Unknown key form
- **WHEN** a passkey declares an algorithm the verification does not know
- **THEN** the verdict is unverified

### Requirement: Reference inventory comparison
When a reference inventory is supplied, the verification SHALL compare item IDs, credential identity, key fingerprint and counter of every passkey, and the content hash of every attachment against it. Any difference SHALL produce a failed finding, except that a passkey counter higher than in the reference SHALL pass, because counters grow with use.

#### Scenario: Swapped credential without reference
- **WHEN** an item's passkey was replaced by a different valid passkey, the count is unchanged and no reference inventory is supplied
- **THEN** no finding is reported for the swap
- **AND** the result lists credential identity as not checked independently

#### Scenario: Swapped credential with reference
- **WHEN** the same capture is verified with a reference inventory that holds the original credential
- **THEN** the verdict is failed
- **AND** the finding names the item ID

#### Scenario: Attachment content differs from reference
- **WHEN** a downloaded attachment's content hash differs from the reference inventory
- **THEN** the verdict is failed

#### Scenario: Counter grew since the reference
- **WHEN** a passkey's counter in the plaintext export is higher than in the reference inventory and everything else matches
- **THEN** no finding is reported for this passkey

#### Scenario: Counter lower than the reference
- **WHEN** a passkey's counter in the plaintext export is lower than in the reference inventory
- **THEN** the verdict is failed
- **AND** the finding names the item ID and the passkey position

### Requirement: Attachment accounting by ID
The verification SHALL expect one download result for every attachment the item list shows on a personal item, identified by item ID and attachment ID. A failed download, a missing result or a result for an attachment the item list does not show SHALL produce a failed finding. File names SHALL NOT be used to identify attachments.

#### Scenario: Two attachments with the same name
- **WHEN** one item has two attachments with the same file name, different attachment IDs and different content
- **THEN** both are accounted for separately with their own content hash
- **AND** the verdict is verified

#### Scenario: One download fails
- **WHEN** the download result for one attachment reports an error
- **THEN** the verdict is failed
- **AND** the finding names the item ID and the attachment ID

#### Scenario: Result missing
- **WHEN** the item list shows an attachment for which no download result exists
- **THEN** the verdict is failed

#### Scenario: Empty attachment
- **WHEN** a download succeeds with zero bytes of content
- **THEN** a warning names the item ID and the attachment ID
- **AND** the verdict is not changed by it

### Requirement: Attachment inventory coverage
The verification SHALL compare the personal items in the item list with the items in the plaintext export. A personal item in the item list that is missing from the plaintext export SHALL produce a failed finding. An exported item that is missing from the item list SHALL produce an unverified finding, because its attachments are unknown.

#### Scenario: Listed item not exported
- **WHEN** the item list shows a personal item that the plaintext export does not contain
- **THEN** the verdict is failed

#### Scenario: Exported item not listed
- **WHEN** the plaintext export contains an item that the item list does not show
- **THEN** the verdict is unverified
- **AND** the finding states that the attachment inventory of this item is unknown

### Requirement: Organization items stay visible
Items in the item list that belong to an organization SHALL be excluded from all comparisons and SHALL be reported in a warning with the number of items per organization ID. An item that both exports hold as a personal item SHALL produce a failed finding when the item list shows it as an organization item, because the documents then disagree about the same snapshot. A download result for an attachment that the item list shows on an organization item SHALL produce a warning.

#### Scenario: Organization items present
- **WHEN** the item list shows three items across two organizations in addition to the personal items
- **THEN** a warning reports three excluded items and the count per organization ID
- **AND** these items are not reported as missing from the export

#### Scenario: Exported item listed as organization item
- **WHEN** both exports contain an item and the item list shows it as an organization item
- **THEN** the verdict is failed
- **AND** the finding names the item ID

#### Scenario: Download result for an organization attachment
- **WHEN** a download result exists for an attachment that the item list shows on an organization item
- **THEN** a warning names the item ID and the attachment ID
- **AND** the verdict is not changed by it

### Requirement: Attributable findings
Every finding SHALL carry a severity, a stable machine-readable reason code and, where applicable, the item ID and attachment ID it refers to.

#### Scenario: Finding for a missing passkey
- **WHEN** a passkey count mismatch is found
- **THEN** the finding carries the severity failed, a reason code for the count mismatch and the item ID

### Requirement: No secrets in results
Results SHALL NOT contain key material, passwords, item names, user names, URIs, notes, field values or attachment file names. Items, passkeys and attachments SHALL be identified by ID, position, count and content hash only.

#### Scenario: Failing capture with sensitive content
- **WHEN** a capture whose items carry known sentinel values in every sensitive field produces failed findings
- **THEN** the serialized result contains none of the sentinel values
