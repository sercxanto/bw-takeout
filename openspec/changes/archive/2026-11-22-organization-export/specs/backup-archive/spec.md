# Spec Delta

## MODIFIED Requirements

### Requirement: Captured documents are stored unchanged
The archive SHALL contain the plaintext export, the account-bound encrypted export, the default item list, the archived item list and, for each chosen organization, its plaintext and its encrypted organization export, exactly as captured, byte for byte. Each of these SHALL be stored as a document of its own. Writing SHALL be refused when one of the four documents or a document of a chosen organization is absent.

#### Scenario: Plaintext export round trip
- **WHEN** an archive is written and read back
- **THEN** the plaintext export read back has the same SHA-256 as the captured one

#### Scenario: Both item lists round trip
- **WHEN** an archive is written from a capture whose default item list and archived item list differ, and is read back
- **THEN** each item list read back has the same SHA-256 as the captured one

#### Scenario: Empty archived item list
- **WHEN** the captured archived item list is an empty JSON array
- **THEN** the archive holds it as a document of its own with exactly these bytes

#### Scenario: Archived item list absent
- **WHEN** writing is requested without an archived item list
- **THEN** the request is refused
- **AND** no archive is finished

#### Scenario: Organization documents round trip
- **WHEN** an archive is written from a capture with two chosen organizations and is read back
- **THEN** each of the four organization documents read back has the same SHA-256 as the captured one

#### Scenario: Organization document absent
- **WHEN** writing is requested for a chosen organization without its encrypted organization export
- **THEN** the request is refused
- **AND** no archive is finished

### Requirement: Entry names come from fixed names and validated IDs
Entry names SHALL be built only from fixed names, from item IDs and organization IDs that have the form of a UUID, and from attachment IDs that consist of 1 to 64 ASCII letters and digits. A capture with any other item ID, organization ID or attachment ID SHALL be refused, and nothing SHALL be published.

#### Scenario: ID with a path separator
- **WHEN** an attachment ID contains a path separator or a parent directory reference
- **THEN** writing is refused
- **AND** no generation is published

#### Scenario: Attachment ID that is not a UUID
- **WHEN** an attachment ID consists of 32 ASCII letters and digits and is not a UUID
- **THEN** writing succeeds and the attachment is stored under that ID

#### Scenario: Organization ID that is not a UUID
- **WHEN** writing is requested for a chosen organization whose ID contains a path separator
- **THEN** writing is refused
- **AND** no generation is published

### Requirement: Manifest
The archive SHALL end with a manifest that states the format version, the creation time, the source server and account, each chosen organization with its ID, its name and the number of items in its plaintext organization export, the tool version and the `bw` version, and lists every other entry with its size and SHA-256. The manifest SHALL embed the verification result unchanged.

#### Scenario: Manifest covers every entry
- **WHEN** an archive is read
- **THEN** every entry other than the manifest appears exactly once in the manifest with matching size and SHA-256

#### Scenario: Verification result embedded unchanged
- **WHEN** an archive is written with a verification result
- **THEN** the result in the manifest is byte-identical to the serialized result that was supplied

#### Scenario: Chosen organization in the manifest
- **WHEN** an archive is written from a capture with one chosen organization whose plaintext organization export holds three items
- **THEN** the manifest lists that organization with its ID, its name and the count three

#### Scenario: No organization chosen
- **WHEN** an archive is written from a capture without a chosen organization
- **THEN** the manifest holds an empty organization list
