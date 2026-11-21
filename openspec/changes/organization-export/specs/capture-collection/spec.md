# Spec Delta

## ADDED Requirements

### Requirement: Chosen organizations are checked before the documents
When organizations are chosen, the collection SHALL read the organization list after the status was accepted and before the first document. Each chosen organization SHALL be in the list with a confirmed membership and the role owner or admin. Otherwise, or when the list cannot be read, the collection SHALL end without a capture. Without chosen organizations the list SHALL NOT be read.

#### Scenario: Admin of the chosen organization
- **WHEN** one organization is chosen and the list shows it with a confirmed membership and the role admin
- **THEN** the collection goes on to the documents

#### Scenario: Member without the role
- **WHEN** one organization is chosen and the list shows it with a confirmed membership and the role user
- **THEN** the collection ends with a failure that names the organization ID and the missing role
- **AND** no document is requested

#### Scenario: Chosen organization not in the list
- **WHEN** one of two chosen organizations is not in the list
- **THEN** the collection ends with a failure that names the ID of that organization and states that the account is not a member
- **AND** no document is requested

#### Scenario: Membership not confirmed
- **WHEN** the list shows the chosen organization with the role owner and a membership that is not confirmed
- **THEN** the collection ends with a failure that names the organization ID and the unconfirmed membership

#### Scenario: Custom role
- **WHEN** the list shows the chosen organization with a custom role
- **THEN** the collection ends with a failure that names the organization ID and the missing role

#### Scenario: No organization chosen
- **WHEN** no organization is chosen
- **THEN** the organization list is not read

#### Scenario: Organization list fails
- **WHEN** the organization list exits with a non-zero exit code
- **THEN** the collection ends with a failure that names the organization list
- **AND** no document is requested

### Requirement: Organization source comes from the organization list
For each chosen organization, the collection SHALL report its ID and the name that the accepted organization list gives as part of the source of the capture, in the order the organizations were chosen. A name the list does not hold SHALL be reported as empty.

#### Scenario: Source with two organizations
- **WHEN** two organizations are chosen and the list gives a name for each
- **THEN** the source names both IDs with their names in the order they were chosen

### Requirement: Two documents per chosen organization are collected
After the four documents, the collection SHALL request for each chosen organization, in the order they were chosen, its plaintext organization export and then its encrypted organization export, each once, and SHALL pass each on byte for byte. A failure of either command SHALL end the collection without a capture with a failure that names the organization ID.

#### Scenario: Two organizations
- **WHEN** two organizations are chosen and every command succeeds
- **THEN** the capture holds the four documents and two documents per organization, unchanged
- **AND** the documents of the first organization were requested before those of the second

#### Scenario: Organization export fails
- **WHEN** the encrypted export of the second chosen organization exits with a non-zero exit code
- **THEN** the collection ends with a failure that names the encrypted organization export and the ID of that organization
- **AND** no attachment is requested

#### Scenario: Session lost on an organization export
- **WHEN** the plaintext export of a chosen organization exits with a non-zero exit code and the status then reports that the client is not logged in
- **THEN** the collection ends with a failure that names the plaintext organization export, the organization ID and the class not logged in
- **AND** no further command on the vault is started

### Requirement: Attachments in scope are downloaded once
The collection SHALL request one download for each attachment that the default or the archived item list shows on a personal item or on an item of a chosen organization, by item ID and attachment ID. An ID pair that both lists show SHALL be requested once. Attachments of items of other organizations SHALL NOT be requested. When an item list is rejected as a document, no attachment SHALL be requested.

#### Scenario: Attachment shown by both lists
- **WHEN** the default and the archived item list both show one personal item with one attachment
- **THEN** that attachment is requested exactly once

#### Scenario: Attachment of a chosen organization
- **WHEN** the item list shows an attachment on an item of a chosen organization
- **THEN** that attachment is requested once

#### Scenario: Attachment of an organization that is not chosen
- **WHEN** the item list shows an attachment on an item of an organization that is not chosen
- **THEN** that attachment is not requested
- **AND** no download result exists for it

#### Scenario: Unparsable item list
- **WHEN** the archived item list is not valid JSON
- **THEN** no attachment is requested
- **AND** the capture is still passed to the verification

## MODIFIED Requirements

### Requirement: Decryption error counts are passed on
The collection SHALL pass on the number of decryption error lines of each of the four documents, of each organization document and of each successful download together with the capture. A count SHALL NOT turn a successful command into a failed one.

#### Scenario: Plaintext export with decryption errors
- **WHEN** the plaintext export exits with code 0 and writes three decryption error lines
- **THEN** the capture carries the count three for the plaintext export

#### Scenario: Organization export with decryption errors
- **WHEN** the plaintext export of a chosen organization exits with code 0 and writes two decryption error lines
- **THEN** the capture carries the count two for the plaintext export of that organization

#### Scenario: Download with decryption errors
- **WHEN** an attachment download succeeds and writes three decryption error lines
- **THEN** its download result is successful and carries the count three

## REMOVED Requirements

### Requirement: Attachments of personal items are downloaded once
**Reason**: The attachments of chosen organizations are downloaded as well, so the name and the rule no longer fit.
**Migration**: See "Attachments in scope are downloaded once", which keeps every rule for personal items and adds the items of chosen organizations.
