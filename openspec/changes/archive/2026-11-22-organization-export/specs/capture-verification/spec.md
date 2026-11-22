# Spec Delta

## ADDED Requirements

### Requirement: Organization exports are validated as documents
For each chosen organization, the verification SHALL reject a plaintext organization export that is not marked as unencrypted, an encrypted organization export that is not marked as encrypted, is protected by a password or carries no key validation, and either document when it is absent or cannot be parsed. Each such finding SHALL be failed and SHALL name the document and the organization ID.

#### Scenario: Password-protected organization export
- **WHEN** the encrypted export of a chosen organization is protected by a password
- **THEN** the verdict is failed
- **AND** the finding names the encrypted organization export and the organization ID

#### Scenario: Organization export absent
- **WHEN** the capture holds no plaintext export for a chosen organization
- **THEN** the verdict is failed
- **AND** the finding names the plaintext organization export and the organization ID

### Requirement: Organization exports are compared like the personal exports
For each chosen organization, the verification SHALL apply the item identity comparison, the passkey count comparison, the rules for unverifiable item formats and the passkey structure validation to its plaintext and its encrypted organization export, with the same severities as for the personal exports. Each finding SHALL name the organization ID.

#### Scenario: Item missing from the plaintext organization export
- **WHEN** an item ID appears in the encrypted export of a chosen organization but not in its plaintext export
- **THEN** the verdict is failed
- **AND** the finding names the item ID and the organization ID

#### Scenario: Blob-format item in an organization export
- **WHEN** the encrypted export of a chosen organization contains a login item in blob format
- **THEN** the verdict is unverified
- **AND** the finding names the item ID and the organization ID

#### Scenario: Passkey with an empty key in an organization export
- **WHEN** a passkey in the plaintext export of a chosen organization has an empty private key
- **THEN** the verdict is failed

#### Scenario: Intact organization
- **WHEN** both exports of a chosen organization hold the same two items with the same passkey counts and valid passkeys
- **THEN** no finding is reported for that organization

### Requirement: Organization items belong to their organization
Every item in either export of a chosen organization SHALL carry the ID of that organization. An item with another organization ID or none SHALL produce a failed finding that names the item ID and the organization ID. So SHALL an item of such an export that the item lists show with another organization ID or as a personal item.

#### Scenario: Item of another organization in the export
- **WHEN** the plaintext export of a chosen organization holds an item with the ID of another organization
- **THEN** the verdict is failed
- **AND** the finding names the item ID and the organization ID of the export

#### Scenario: Exported item listed as personal item
- **WHEN** both exports of a chosen organization hold an item and the item list shows it as a personal item
- **THEN** the verdict is failed
- **AND** the finding names the item ID

### Requirement: Organization exports agree on revisions
When an item ID is in both exports of a chosen organization, the verification SHALL compare the revision dates of the two entries. A difference SHALL produce a failed finding that names the item ID and the organization ID and states that the organization changed during the capture.

#### Scenario: Item changed between the two exports
- **WHEN** an item of a chosen organization has a later revision date in the encrypted export than in the plaintext export
- **THEN** the verdict is failed
- **AND** the finding states that the organization changed during the capture

#### Scenario: Same revisions
- **WHEN** every item of a chosen organization has the same revision date in both exports
- **THEN** no finding about revisions is reported

### Requirement: Items of chosen organizations are matched with the item lists
For each chosen organization, the verification SHALL compare the items the item lists show for it with its plaintext export. A listed item missing from the export SHALL produce a failed finding. An exported item missing from the lists SHALL produce an unverified finding, because its attachments are unknown. An item in both with different revision dates SHALL produce a failed finding that states the organization changed.

#### Scenario: Listed organization item not exported
- **WHEN** the item list shows an item of a chosen organization that its plaintext export does not contain
- **THEN** the verdict is failed
- **AND** the finding names the item ID and the organization ID

#### Scenario: Exported organization item not listed
- **WHEN** the plaintext export of a chosen organization contains an item that neither item list shows
- **THEN** the verdict is unverified
- **AND** the finding states that the attachment inventory of this item is unknown

#### Scenario: Item changed after the snapshot
- **WHEN** the item list shows an item of a chosen organization with an earlier revision date than its plaintext export
- **THEN** the verdict is failed
- **AND** the finding states that the organization changed during the capture

### Requirement: Organization items outside the scope
Items of an organization that is not chosen SHALL be excluded from every comparison with the exports and from the attachment accounting, and SHALL produce no finding and no warning apart from the checks of the item lists as documents. An item that both personal exports hold as a personal item SHALL produce a failed finding when the item list shows it as an organization item. A download result for an attachment on an item of an organization that is not chosen SHALL produce a warning.

#### Scenario: Items of organizations that are not chosen
- **WHEN** the item list shows three items across two organizations that are not chosen, in addition to the personal items
- **THEN** no finding and no warning refers to these items or organizations
- **AND** these items are not reported as missing from any export

#### Scenario: Exported item listed as organization item
- **WHEN** both personal exports contain an item and the item list shows it as an organization item
- **THEN** the verdict is failed
- **AND** the finding names the item ID

#### Scenario: Download result for an attachment outside the scope
- **WHEN** a download result exists for an attachment that the item list shows on an item of an organization that is not chosen
- **THEN** a warning names the item ID and the attachment ID
- **AND** the verdict is not changed by it

### Requirement: Item lists are checked as documents in every scope
The checks of the default and the archived item list as documents SHALL cover every item the lists show, also an item of an organization that is not chosen: a duplicate ID within one list, the comparison of an item that both lists show, and the warning that the archive filter did not apply.

#### Scenario: Unchosen organization item in both lists
- **WHEN** both item lists show the same item of an organization that is not chosen, with the same attachments
- **THEN** the warning that the archive filter did not apply counts this item
- **AND** no other finding or warning refers to it

#### Scenario: Unchosen organization item that differs between the lists
- **WHEN** both item lists show the same item of an organization that is not chosen, with different attachments
- **THEN** the verdict is failed
- **AND** the finding names the item ID

## MODIFIED Requirements

### Requirement: Scope statement and standing limits
Every result SHALL state the scope its verdict refers to: the personal items present in the captured local snapshot and, for each chosen organization by its ID, the items of its organization exports. A result SHALL NOT claim that all items on the server are covered. Every result SHALL list the checks that were not possible for this capture. A result with a chosen organization SHALL list as a limit that the organization exports are separate reads from the server.

#### Scenario: Verified result without reference inventory
- **WHEN** a capture is verified and no reference inventory was supplied
- **THEN** the result names the scope
- **AND** the result states that credential identity and attachment content were not checked independently

#### Scenario: Scope with a chosen organization
- **WHEN** a capture with one chosen organization is verified
- **THEN** the scope names the personal items and the ID of that organization
- **AND** the result lists the limit that the organization exports are separate reads from the server

#### Scenario: No organization chosen
- **WHEN** a capture without a chosen organization is verified
- **THEN** the scope names the personal items only
- **AND** the result does not list the limit about organization exports

### Requirement: Attachment accounting by ID
The verification SHALL expect one download result for every attachment the item list shows on a personal item or on an item of a chosen organization, identified by item ID and attachment ID. A failed download, a missing result or a result for an attachment the item list does not show SHALL produce a failed finding. File names SHALL NOT be used to identify attachments.

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

#### Scenario: Result missing for a chosen organization
- **WHEN** the item list shows an attachment on an item of a chosen organization for which no download result exists
- **THEN** the verdict is failed
- **AND** the finding names the item ID and the attachment ID

#### Scenario: Empty attachment
- **WHEN** a download succeeds with zero bytes of content
- **THEN** a warning names the item ID and the attachment ID
- **AND** the verdict is not changed by it

### Requirement: Reference inventory comparison
When a reference inventory is supplied, the verification SHALL compare against it the item IDs of the personal exports and of the plaintext exports of the chosen organizations, the credential identity, key fingerprint and counter of every passkey, and the content hash of every attachment. Any difference SHALL produce a failed finding, except that a passkey counter higher than in the reference SHALL pass, because counters grow with use.

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

#### Scenario: Organization item missing from the capture
- **WHEN** the reference inventory holds an item of a chosen organization that its plaintext export does not contain
- **THEN** the verdict is failed
- **AND** the finding names the item ID

#### Scenario: Counter grew since the reference
- **WHEN** a passkey's counter in the plaintext export is higher than in the reference inventory and everything else matches
- **THEN** no finding is reported for this passkey

#### Scenario: Counter lower than the reference
- **WHEN** a passkey's counter in the plaintext export is lower than in the reference inventory
- **THEN** the verdict is failed
- **AND** the finding names the item ID and the passkey position

### Requirement: Attributable findings
Every finding SHALL carry a severity, a stable machine-readable reason code and, where applicable, the organization ID, the item ID and the attachment ID it refers to.

#### Scenario: Finding for a missing passkey
- **WHEN** a passkey count mismatch is found
- **THEN** the finding carries the severity failed, a reason code for the count mismatch and the item ID

#### Scenario: Finding about an organization export
- **WHEN** a passkey count mismatch is found in the exports of a chosen organization
- **THEN** the finding carries the organization ID in addition to the item ID

### Requirement: Default and archived item lists form one listing
A capture SHALL hold the default item list and the archived item list as two separate documents. The verification SHALL apply every requirement that refers to the item list to the items of both lists together. An item that both lists show SHALL count once. Each list SHALL be validated as a document of its own, and a finding about a document SHALL name the list it refers to.

#### Scenario: Archived item with an attachment
- **WHEN** both exports contain an archived item, only the archived item list shows it with one attachment, and a successful download result exists for that attachment
- **THEN** no finding is reported for this item
- **AND** the verdict is verified

#### Scenario: Archived item in neither list
- **WHEN** the plaintext export contains an archived item that neither list shows
- **THEN** the verdict is unverified
- **AND** the finding states that the attachment inventory of this item is unknown

#### Scenario: Download missing for an archived item
- **WHEN** the archived item list shows an attachment for which no download result exists
- **THEN** the verdict is failed
- **AND** the finding names the item ID and the attachment ID

#### Scenario: Duplicate ID within the archived item list
- **WHEN** the archived item list contains the same item ID twice
- **THEN** the verdict is failed
- **AND** the finding names the archived item list as the document

#### Scenario: Archived item list cannot be parsed
- **WHEN** the archived item list is not valid JSON or is absent from the capture
- **THEN** the verdict is failed
- **AND** the finding names the archived item list as the document

#### Scenario: Organization item in both lists
- **WHEN** both lists show the same item of a chosen organization with one attachment
- **THEN** one download result is expected for that attachment
- **AND** the item is matched with the organization export once

### Requirement: Decryption error lines are reported
A capture whose plaintext export, encrypted export, default item list, archived item list or organization document came with one or more decryption error lines SHALL produce an unverified finding for each such document, naming the document, for an organization document its organization ID, and the number of lines. A successful attachment download with one or more decryption error lines SHALL produce a warning naming the item ID, the attachment ID and the number of lines.

#### Scenario: Decryption errors on the plaintext export
- **WHEN** an otherwise intact capture carries three decryption error lines for the plaintext export
- **THEN** the verdict is unverified
- **AND** the finding names the plaintext export and the count three

#### Scenario: Decryption errors on an organization export
- **WHEN** an otherwise intact capture carries two decryption error lines for the plaintext export of a chosen organization
- **THEN** the verdict is unverified
- **AND** the finding names the plaintext organization export, the organization ID and the count two

#### Scenario: Decryption errors together with a proven deviation
- **WHEN** a capture carries decryption error lines for the item list and a passkey count mismatch
- **THEN** the verdict is failed
- **AND** both findings are listed

#### Scenario: Decryption errors on an attachment download
- **WHEN** an otherwise intact capture has a successful download that carries three decryption error lines
- **THEN** a warning names the item ID, the attachment ID and the count three
- **AND** the verdict is verified

#### Scenario: No decryption errors
- **WHEN** every count is zero
- **THEN** no finding about decryption errors is produced

## REMOVED Requirements

### Requirement: Organization items stay visible
**Reason**: Organizations are now chosen explicitly. Items of organizations that are not chosen are outside the scope and produce no warning (maintainer decision, 21 November 2026).
**Migration**: See "Organization items outside the scope", which keeps the failed finding for a personal item listed as an organization item and the warning for a download outside the scope, and drops the warning with counts per organization. Items of chosen organizations follow "Items of chosen organizations are matched with the item lists".
