# Spec Delta

## MODIFIED Requirements

### Requirement: Unsafe or unknown archive structure is rejected
The verify command SHALL accept only regular file entries with the names the format defines. Any other entry type, an unexpected name, a duplicate entry, recovery instructions that are not the first entry, a manifest that is not the last entry, an absent entry that the format requires and an unknown format version SHALL each make the verification fail. The format requires the recovery instructions, the plaintext export, the account-bound encrypted export, the default item list and the archived item list.

#### Scenario: Symbolic link entry
- **WHEN** an archive contains a symbolic link entry
- **THEN** the verification fails

#### Scenario: Recovery instructions not first
- **WHEN** the recovery instructions are not the first entry of the archive
- **THEN** the verification fails and names the entry

#### Scenario: Newer format version
- **WHEN** the manifest states a format version the command does not know
- **THEN** the verification fails and states that the version is not supported

#### Scenario: Archive without the archived item list
- **WHEN** an otherwise consistent archive holds no archived item list and its manifest lists none
- **THEN** the verification fails and names the missing entry
