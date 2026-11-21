# Spec Delta

## ADDED Requirements

### Requirement: Organization entries follow the manifest
For each organization the manifest lists, the archive SHALL hold its plaintext and its encrypted organization export, and the item count the manifest states for it SHALL match its plaintext organization export. A missing organization document, a document of an organization the manifest does not list and a count that differs SHALL each make the verification fail and SHALL be named by the entry name or the organization ID.

#### Scenario: Intact generation with an organization
- **WHEN** an intact generation with one organization is verified with a matching identity
- **THEN** the verification succeeds

#### Scenario: Organization document missing
- **WHEN** the manifest lists an organization and the archive holds no encrypted export for it
- **THEN** the verification fails and names the missing entry

#### Scenario: Document of an organization that is not listed
- **WHEN** the archive holds an organization document whose organization the manifest does not list
- **THEN** the verification fails and names the entry

#### Scenario: Organization item count differs
- **WHEN** the manifest states two items for an organization whose plaintext organization export holds three
- **THEN** the verification fails and names the organization ID

### Requirement: The manifest lists the organizations
The verify command SHALL require the manifest to hold an organization list, which is empty when no organization was chosen. A manifest without it, such as one written before organizations were supported, SHALL make the verification fail.

#### Scenario: Generation without an organization
- **WHEN** an intact generation whose manifest holds an empty organization list is verified with a matching identity
- **THEN** the verification succeeds
- **AND** the output shows no organization

#### Scenario: Manifest without an organization list
- **WHEN** a generation whose manifest holds no organization list is verified
- **THEN** the verification fails and states that the manifest is invalid

## MODIFIED Requirements

### Requirement: The stored verdict is reported
The verify command SHALL print the creation time, the source server and account, the number of items and attachments, the ID and the number of items of each listed organization, and the stored verdict with the number of findings and warnings. Its exit status SHALL distinguish an intact generation with a verified capture, an intact generation with an unverified capture, and a failed verification.

#### Scenario: Verified capture
- **WHEN** an intact generation with the stored verdict verified is checked
- **THEN** the output shows the verdict verified and the exit status is 0

#### Scenario: Unverified capture
- **WHEN** an intact generation with the stored verdict unverified is checked
- **THEN** the output shows the verdict unverified and the number of findings
- **AND** the exit status is 3

#### Scenario: Failed verification
- **WHEN** the verification fails for any reason
- **THEN** the exit status is 1

#### Scenario: Generation with an organization
- **WHEN** an intact generation with one organization of three items is checked
- **THEN** the output shows the organization ID and the count three
- **AND** the output does not show the organization name
