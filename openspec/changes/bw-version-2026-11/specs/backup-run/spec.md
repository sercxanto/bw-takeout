# Spec Delta

## MODIFIED Requirements

### Requirement: Report holds no vault content and no bw text
The report SHALL NOT contain item names, user names, passwords, notes, field values, key material, attachment file names, organization names, the session key or text that `bw` wrote. The one exception is the key path of a warning about an export key unknown to this project: it MAY contain key names of the export that have the form of an identifier, an ASCII letter followed by at most 63 ASCII letters or digits, and no other text of the export. Characters that are not printable SHALL be replaced in every value it prints. The prompts of `bw` during the login reach the terminal directly and are not part of the report.

#### Scenario: Sentinels in the vault and in bw output
- **WHEN** the vault holds sentinel values in names, notes, passwords and file names, and every `bw` command other than the login writes a sentinel to standard error
- **THEN** no sentinel appears in the report on standard output or in the messages of the run on standard error

#### Scenario: Sentinel in an organization name
- **WHEN** a chosen organization has a sentinel value as its name and the run publishes a generation
- **THEN** no sentinel appears in the report on standard output or in the messages of the run on standard error

#### Scenario: Control characters in the account address
- **WHEN** the status reports an account address with a terminal escape sequence
- **THEN** the report prints it with the control characters replaced

#### Scenario: Key path of an unknown export key
- **WHEN** an item of the plaintext export carries an unknown key of identifier form with a sentinel value, and an unknown key that contains a sentinel and is not an identifier
- **THEN** the report names the path of the identifier key in a warning, and the other key only as the placeholder
- **AND** no sentinel appears in the report on standard output or in the messages of the run on standard error
