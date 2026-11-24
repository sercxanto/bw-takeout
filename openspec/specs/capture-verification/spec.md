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

### Requirement: Attachment inventory coverage
The verification SHALL compare the personal items in the item list with the items in the plaintext export. A personal item in the item list that is missing from the plaintext export SHALL produce a failed finding. An exported item that is missing from the item list SHALL produce an unverified finding, because its attachments are unknown.

#### Scenario: Listed item not exported
- **WHEN** the item list shows a personal item that the plaintext export does not contain
- **THEN** the verdict is failed

#### Scenario: Exported item not listed
- **WHEN** the plaintext export contains an item that the item list does not show
- **THEN** the verdict is unverified
- **AND** the finding states that the attachment inventory of this item is unknown

### Requirement: Attributable findings
Every finding SHALL carry a severity, a stable machine-readable reason code and, where applicable, the organization ID, the item ID and the attachment ID it refers to.

#### Scenario: Finding for a missing passkey
- **WHEN** a passkey count mismatch is found
- **THEN** the finding carries the severity failed, a reason code for the count mismatch and the item ID

#### Scenario: Finding about an organization export
- **WHEN** a passkey count mismatch is found in the exports of a chosen organization
- **THEN** the finding carries the organization ID in addition to the item ID

### Requirement: No secrets in results
Results SHALL NOT contain key material, passwords, item names, user names, URIs, notes, field values or attachment file names. Items, passkeys and attachments SHALL be identified by ID, position, count and content hash only.

#### Scenario: Failing capture with sensitive content
- **WHEN** a capture whose items carry known sentinel values in every sensitive field produces failed findings
- **THEN** the serialized result contains none of the sentinel values

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

### Requirement: Item shown by both lists must agree
When the default item list and the archived item list show the same item ID, the verification SHALL compare the attachment IDs and the organization ID of the two entries. A difference SHALL produce a failed finding that names the item ID, because the documents then disagree about the same snapshot. Two entries that agree SHALL NOT produce a finding for the item.

#### Scenario: Same item with the same attachments
- **WHEN** both lists show an item with the same two attachment IDs
- **THEN** no finding is reported for this item
- **AND** one download result per attachment is expected

#### Scenario: Same item with different attachments
- **WHEN** both lists show an item, and one entry names an attachment ID that the other does not
- **THEN** the verdict is failed
- **AND** the finding names the item ID

#### Scenario: Same item with a different organization
- **WHEN** one list shows an item as a personal item and the other shows it as an organization item
- **THEN** the verdict is failed
- **AND** the finding names the item ID

### Requirement: Archive filter that did not apply is reported
The verification SHALL report a warning when at least one item ID appears in both the default item list and the archived item list, because the official CLI then did not apply the archive filter. The warning SHALL state the number of item IDs that both lists show. It SHALL NOT change the verdict.

#### Scenario: Archived list repeats the default list
- **WHEN** the archived item list holds the same items as the default item list and every entry agrees
- **THEN** a warning reports that the archive filter did not apply and gives the number of shared item IDs
- **AND** the verdict is verified

#### Scenario: Disjoint lists
- **WHEN** no item ID appears in both lists
- **THEN** no such warning is reported

#### Scenario: Both lists empty
- **WHEN** both lists are empty and both exports hold no item
- **THEN** no such warning is reported
- **AND** the verdict is verified

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

### Requirement: Export keys unknown to this project are reported
The verification SHALL compare the keys of the plaintext and the encrypted export, personal and of each chosen organization, with the keys this project knows for their forms. Each unknown key path SHALL produce one warning that names the document, the organization ID where it applies, the path and the number of items carrying it. The value of an unknown key SHALL NOT be inspected. Warnings SHALL NOT change the verdict.

#### Scenario: Unknown key on an item
- **WHEN** an otherwise intact capture has a plaintext export in which one item carries a key this project does not know
- **THEN** a warning names the plaintext export, the key path and the count one
- **AND** the verdict is verified

#### Scenario: Same unknown key on several items
- **WHEN** three items of the encrypted export carry the same unknown key at the same place
- **THEN** exactly one warning is reported for that key path, with the count three

#### Scenario: Unknown key inside a passkey of an organization export
- **WHEN** a passkey in the plaintext export of a chosen organization carries a key this project does not know
- **THEN** a warning names the plaintext organization export, the organization ID, the key path below the passkey and the count one

#### Scenario: Unknown key at document level
- **WHEN** the plaintext export carries a top-level key next to encrypted, folders and items that this project does not know
- **THEN** a warning names the plaintext export and that key path

#### Scenario: Keys within an unknown value
- **WHEN** an unknown key of an item holds an object with further keys
- **THEN** only the unknown key itself is reported
- **AND** no warning names a key inside its value

#### Scenario: Known keys only
- **WHEN** every key of both exports is one this project knows, including the data key of a blob-format item and the key of a legacy item
- **THEN** no warning about unknown keys is reported

#### Scenario: Known key absent
- **WHEN** an item of the plaintext export lacks a key this project knows
- **THEN** no warning about unknown keys is reported for it

### Requirement: Item types unknown to this project are reported
The verification SHALL produce one warning per document and unknown item type number when an item of the plaintext export, personal or of a chosen organization, has an integer type this project does not know. The warning SHALL name the document, the organization ID where it applies, the type number and the number of items. The warning SHALL NOT change the verdict. In the encrypted exports such an item stays an unrecognized format.

#### Scenario: New item type
- **WHEN** two items have the type number 9 in the plaintext export, and the same two items are in the encrypted export
- **THEN** one warning names the plaintext export, the type number 9 and the count two
- **AND** the two items are reported as unverified because their format in the encrypted export is not recognized

#### Scenario: New item type in an organization export
- **WHEN** one item of the plaintext export of a chosen organization has the type number 9
- **THEN** one warning names the plaintext organization export, the organization ID, the type number 9 and the count one

#### Scenario: Known item types
- **WHEN** every item of both exports has one of the type numbers this project knows
- **THEN** no warning about unknown item types is reported

### Requirement: Reported key paths hold no content
A key path in a warning SHALL consist only of keys that have the form of an identifier: an ASCII letter followed by at most 63 ASCII letters or digits. Any other unknown key SHALL be reported under one fixed placeholder for its place, and its text SHALL NOT appear in the result. For each document, at most 32 distinct key paths SHALL be reported, and one more warning SHALL state how many were left out.

#### Scenario: Unknown key that is not an identifier
- **WHEN** an item of the plaintext export carries an unknown key that contains a sentinel value with spaces and an at sign
- **THEN** the warning names the placeholder at that place
- **AND** the serialized result does not contain the sentinel value

#### Scenario: Many distinct unknown keys
- **WHEN** the plaintext export carries 40 distinct unknown identifier keys
- **THEN** 32 warnings name a key path each
- **AND** one more warning states that 8 key paths were left out
- **AND** the verdict is not changed by them
