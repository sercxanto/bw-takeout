# Spec Delta

## ADDED Requirements

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
- **WHEN** both lists show the same organization item
- **THEN** the warning about excluded organization items counts it once

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
