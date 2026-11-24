# Spec Delta

## MODIFIED Requirements

### Requirement: Failures carry a class and no CLI text
A failed command SHALL be reported with the operation, a class from a fixed list and the exit code. Text that `bw` wrote to standard output or standard error SHALL NOT be copied into a failure, a result or any output of this project. The one exception is the key path of a warning about an export key unknown to this project: it MAY contain key names of an export document that have the form of an identifier, an ASCII letter followed by at most 63 ASCII letters or digits, and no other text of the export (capture-verification "Reported key paths hold no content", backup-run "Report holds no vault content and no bw text"). The classes not logged in and locked SHALL be taken from the status document. Text of `bw` SHALL select a class only for premium required on an attachment download. No class SHALL make a result more favorable than a general failure.

#### Scenario: Failed download with a name in the error text
- **WHEN** an attachment download fails and the error text of `bw` contains a sentinel value
- **THEN** the reported failure contains the item ID and the attachment ID
- **AND** it does not contain the sentinel value

#### Scenario: Not logged in
- **WHEN** a vault command fails in a profile without login and the status afterwards reports unauthenticated
- **THEN** the failure has the class not logged in

#### Scenario: Not-logged-in text with an unlocked vault
- **WHEN** a vault command fails with the text "You are not logged in." and the status afterwards reports unlocked
- **THEN** the failure has the general class for a failed command

#### Scenario: Status query after a failure fails
- **WHEN** a vault command fails and the status query that follows fails as well
- **THEN** the failure has the general class for a failed command

#### Scenario: Premium required
- **WHEN** an attachment download ends with the message that premium status is required
- **THEN** the failure has the class premium required

#### Scenario: Unrecognized error text
- **WHEN** a command exits with code 1 and an error text that is not recognized, and the status afterwards reports unlocked
- **THEN** the failure has the general class for a failed command
