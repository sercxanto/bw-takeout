# Spec Delta

## ADDED Requirements

### Requirement: Organization exports are requested by ID
An organization export SHALL be requested by organization ID only, in the plaintext or the encrypted form. The ID SHALL have the form of a UUID before it is used as an argument. An ID of another form SHALL be refused before any process is started. A failed organization export SHALL name the organization ID.

#### Scenario: Export by organization ID
- **WHEN** the encrypted export of an organization is requested with a valid organization ID
- **THEN** the process is asked for the encrypted export of exactly this organization
- **AND** the document is returned from memory

#### Scenario: Organization ID that starts with a dash
- **WHEN** an organization export is requested with an ID that starts with a dash
- **THEN** the request is refused
- **AND** no `bw` process is started

#### Scenario: Failed organization export with a name in the error text
- **WHEN** an organization export fails and the error text of `bw` contains a sentinel value
- **THEN** the reported failure contains the organization ID
- **AND** it does not contain the sentinel value

### Requirement: Organization list is read from its output
The organization list SHALL count as successful only when its output is a JSON array whose every entry holds an organization ID of UUID form, a numeric role and a numeric membership status. Any other output SHALL be reported as failed. The name of an entry SHALL be passed on as it is, or as empty when it is absent.

#### Scenario: Two memberships
- **WHEN** the organization list exits with code 0 and prints two entries with ID, role, status and name
- **THEN** both entries are returned with their ID, role, status and name

#### Scenario: Entry without a role
- **WHEN** the organization list exits with code 0 and one entry holds no role
- **THEN** the command is reported as failed

#### Scenario: No membership
- **WHEN** the organization list exits with code 0 and prints an empty array
- **THEN** an empty list is returned
