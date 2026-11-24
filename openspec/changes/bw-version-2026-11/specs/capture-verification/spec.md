# Spec Delta

## ADDED Requirements

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
