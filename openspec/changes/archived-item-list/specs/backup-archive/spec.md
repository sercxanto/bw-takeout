# Spec Delta

## MODIFIED Requirements

### Requirement: Captured documents are stored unchanged
The archive SHALL contain the plaintext export, the account-bound encrypted export, the default item list and the archived item list exactly as captured, byte for byte. The two item lists SHALL be stored as separate documents. Writing SHALL be refused when one of the four documents is absent.

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
