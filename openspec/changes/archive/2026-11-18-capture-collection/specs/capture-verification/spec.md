# Spec Delta

## ADDED Requirements

### Requirement: Decryption error lines are reported
A capture whose plaintext export, encrypted export, default item list or archived item list came with one or more decryption error lines SHALL produce an unverified finding for each such document, naming the document and the number of lines. A successful attachment download with one or more decryption error lines SHALL produce a warning naming the item ID, the attachment ID and the number of lines.

#### Scenario: Decryption errors on the plaintext export
- **WHEN** an otherwise intact capture carries three decryption error lines for the plaintext export
- **THEN** the verdict is unverified
- **AND** the finding names the plaintext export and the count three

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
