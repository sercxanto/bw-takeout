# Spec Delta

## Purpose

Lets a user confirm that a published generation can be opened with a separately stored key and is internally consistent, on a system without network access and without the Bitwarden CLI, and shows which verdict the generation carries.

## ADDED Requirements

### Requirement: Verification works offline and without bw
The verify command SHALL need only a generation and an age identity file. It SHALL NOT use the network and SHALL NOT start `bw` or any other program.

#### Scenario: System without bw
- **WHEN** the verify command runs with an empty program search path on an intact generation
- **THEN** it succeeds

### Requirement: Entries are checked against the manifest
The verify command SHALL compare the size and SHA-256 of every archive entry with the manifest. An entry that differs, an entry the manifest does not list and a listed entry that is missing SHALL each make the verification fail and SHALL be named by its entry name. The item and attachment counts of the manifest SHALL match the plaintext export and the listed attachments.

#### Scenario: Intact generation
- **WHEN** an intact generation is verified with a matching identity
- **THEN** the verification succeeds

#### Scenario: Entry differs from the manifest
- **WHEN** an archive holds an attachment whose content does not match the manifest
- **THEN** the verification fails and names the entry

#### Scenario: Entry not listed
- **WHEN** an archive holds an entry that the manifest does not list
- **THEN** the verification fails and names the entry

#### Scenario: Listed entry missing
- **WHEN** the manifest lists an entry that the archive does not hold
- **THEN** the verification fails and names the entry

#### Scenario: Item count differs from the export
- **WHEN** the manifest counts a different number of items than the plaintext export holds
- **THEN** the verification fails

### Requirement: Damaged archives fail as a whole
A truncated or altered archive file SHALL make the verification fail. The command SHALL NOT report a partial success.

#### Scenario: Truncated archive file
- **WHEN** the archive file was cut off before its end
- **THEN** the verification fails

#### Scenario: Altered byte
- **WHEN** one byte of the archive file was changed
- **THEN** the verification fails

### Requirement: Wrong identity is reported clearly
When no supplied identity matches a recipient of the archive, the verification SHALL fail and say that the identity does not match.

#### Scenario: Identity of another key pair
- **WHEN** a generation is verified with an identity it was not encrypted to
- **THEN** the verification fails with the reason that no identity matches

### Requirement: Incomplete generations are refused
The verify command SHALL refuse a generation that carries the partial marker or holds no archive file, and SHALL report it as incomplete.

#### Scenario: Partial generation
- **WHEN** the verify command is pointed at a directory with the `.partial` marker
- **THEN** it fails and reports the generation as incomplete

#### Scenario: Partial generation reached through another path
- **WHEN** the verify command is given `.` from inside a partial generation, or a symbolic link to one
- **THEN** it fails and reports the generation as incomplete

### Requirement: Unsafe or unknown archive structure is rejected
The verify command SHALL accept only regular file entries with the names the format defines. Any other entry type, an unexpected name, a duplicate entry, recovery instructions that are not the first entry, a manifest that is not the last entry and an unknown format version SHALL each make the verification fail.

#### Scenario: Symbolic link entry
- **WHEN** an archive contains a symbolic link entry
- **THEN** the verification fails

#### Scenario: Recovery instructions not first
- **WHEN** the recovery instructions are not the first entry of the archive
- **THEN** the verification fails and names the entry

#### Scenario: Newer format version
- **WHEN** the manifest states a format version the command does not know
- **THEN** the verification fails and states that the version is not supported

### Requirement: The stored verdict is reported
The verify command SHALL print the creation time, the source server and account, the number of items and attachments, and the stored verdict with the number of findings and warnings. Its exit status SHALL distinguish an intact generation with a verified capture, an intact generation with an unverified capture, and a failed verification.

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

### Requirement: No plaintext files and no vault content in output
The verify command SHALL NOT write any file. Its output SHALL NOT contain item names, user names or passwords of items, key material, notes, field values or attachment file names. The source server and account of the generation are not vault content.

#### Scenario: Generation with sentinel values
- **WHEN** a generation whose sensitive fields carry sentinel values is verified, once intact and once with a damaged entry
- **THEN** neither output contains a sentinel value
- **AND** the temporary directory of the process and the target are unchanged
