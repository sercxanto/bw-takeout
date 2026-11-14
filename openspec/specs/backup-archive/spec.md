# backup-archive Specification

## Purpose

Defines what one backup generation contains and how it is protected: the captured vault data, its attachments, a manifest and recovery instructions inside a single encrypted archive that standard tools can open without this project.

## Requirements

### Requirement: One encrypted archive per generation
A generation SHALL consist of exactly one encrypted archive file. All captured data, file names, the manifest and the recovery instructions SHALL be inside the encryption. Nothing outside the encryption SHALL reveal vault content, item names or attachment file names.

#### Scenario: Content of a generation
- **WHEN** a generation is written from a capture
- **THEN** its directory holds exactly one file, the encrypted archive
- **AND** no file or directory name in the target contains an item name, an attachment file name or an ID from the vault

### Requirement: Recoverable with standard tools
An archive SHALL be decryptable with a standard age implementation and unpackable with a standard tar implementation, without this project's software.

#### Scenario: Recovery with the age and tar commands
- **WHEN** an archive is decrypted with the age command and a matching identity and the result is unpacked with the tar command
- **THEN** the unpacked documents and attachments are byte-identical to the captured ones

### Requirement: Encryption to recipient keys
An archive SHALL be encrypted to one or more age recipient keys. Writing SHALL require only the public recipient keys. Each matching identity alone SHALL open the archive. Writing without a recipient SHALL be refused.

#### Scenario: Two recipients
- **WHEN** an archive is written for two recipients
- **THEN** the identity of the first recipient alone opens it
- **AND** the identity of the second recipient alone opens it

#### Scenario: No recipient
- **WHEN** writing is requested without any recipient
- **THEN** the request is refused
- **AND** nothing is created in the target

#### Scenario: Unsupported recipient
- **WHEN** a recipient string is not a native age recipient key
- **THEN** the request is refused and names the position of the rejected recipient, not its value

### Requirement: Captured documents are stored unchanged
The archive SHALL contain the plaintext export, the account-bound encrypted export and the item list exactly as captured, byte for byte.

#### Scenario: Plaintext export round trip
- **WHEN** an archive is written and read back
- **THEN** the plaintext export read back has the same SHA-256 as the captured one

### Requirement: Attachments are stored by ID
Each attachment SHALL be stored under a name built from its item ID and its attachment ID. The original file name SHALL appear only in the manifest.

#### Scenario: Two attachments with the same file name
- **WHEN** one item has two attachments with the same file name, different IDs and different content
- **THEN** the archive holds two separate entries with the respective content
- **AND** the manifest gives the same file name for both entries

### Requirement: Entry names come from fixed names and validated IDs
Entry names SHALL be built only from fixed names, from item IDs that have the form of a UUID, and from attachment IDs that consist of 1 to 64 ASCII letters and digits. A capture with any other item ID or attachment ID SHALL be refused, and nothing SHALL be published.

#### Scenario: ID with a path separator
- **WHEN** an attachment ID contains a path separator or a parent directory reference
- **THEN** writing is refused
- **AND** no generation is published

#### Scenario: Attachment ID that is not a UUID
- **WHEN** an attachment ID consists of 32 ASCII letters and digits and is not a UUID
- **THEN** writing succeeds and the attachment is stored under that ID

### Requirement: Manifest
The archive SHALL end with a manifest that states the format version, the creation time, the source server and account, the tool version and the `bw` version, and lists every other entry with its size and SHA-256. The manifest SHALL embed the verification result unchanged.

#### Scenario: Manifest covers every entry
- **WHEN** an archive is read
- **THEN** every entry other than the manifest appears exactly once in the manifest with matching size and SHA-256

#### Scenario: Verification result embedded unchanged
- **WHEN** an archive is written with a verification result
- **THEN** the result in the manifest is byte-identical to the serialized result that was supplied

### Requirement: Recovery instructions inside the archive
The archive SHALL begin with a text file that explains the layout, how entries map to items and attachments, and how the documents can be used without this project. The file SHALL contain no data from the vault.

#### Scenario: Instructions present and free of vault data
- **WHEN** an archive is written from a capture whose sensitive fields carry sentinel values
- **THEN** the first entry is the recovery instructions
- **AND** it contains none of the sentinel values

### Requirement: No plaintext files while writing
Writing an archive SHALL NOT create any file with unencrypted captured data, in the target or in any temporary location.

#### Scenario: Temporary location stays empty
- **WHEN** an archive is written while the temporary directory of the process points to an empty directory
- **THEN** that directory is still empty afterwards
- **AND** the target holds no file other than the encrypted archive
