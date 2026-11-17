# Spec Delta

## Purpose

Collects one capture of a personal vault from a logged-in official CLI: a fresh local snapshot, the four captured documents and the attachments of personal items, handed on one at a time, with download results and decryption error counts the verification can judge.

## ADDED Requirements

### Requirement: Collection starts with a forced sync
The collection SHALL start with a forced sync before any document is requested. A failure of the sync SHALL end the collection without a capture.

#### Scenario: Sync fails
- **WHEN** the forced sync exits with a non-zero exit code
- **THEN** the collection ends with a failure that names the sync
- **AND** no document is requested

### Requirement: Snapshot must be fresh
After the sync, the collection SHALL query the status and SHALL accept it only when the state is unlocked and the last sync time is at or after the moment the sync was started, compared at millisecond precision. Any other status SHALL end the collection without a capture.

#### Scenario: Fresh snapshot
- **WHEN** the status after the sync reports unlocked and a last sync time 40 milliseconds after the sync was started
- **THEN** the collection goes on to the documents

#### Scenario: Stale snapshot
- **WHEN** the sync exits with code 0 and the status reports a last sync time before the sync was started
- **THEN** the collection ends with a failure that states the snapshot is not fresh
- **AND** no document is requested

#### Scenario: No last sync time
- **WHEN** the status reports no last sync time
- **THEN** the collection ends without a capture

#### Scenario: Vault not unlocked
- **WHEN** the status after the sync reports the state locked
- **THEN** the collection ends without a capture

### Requirement: Source comes from the status
The collection SHALL report the server URL, the account e-mail address and the account ID from the accepted status as the source of the capture. A value the status does not hold SHALL be reported as empty.

#### Scenario: Source of a capture
- **WHEN** the accepted status names a server URL, an e-mail address and a user ID
- **THEN** the collection reports these three values as the source

### Requirement: Four documents are collected
The collection SHALL request the plaintext export, the account-bound encrypted export, the default item list and the archived item list, each once, and SHALL pass each on byte for byte. A failure of any of these commands SHALL end the collection without a capture.

#### Scenario: All documents collected
- **WHEN** all four commands succeed
- **THEN** the capture holds the four outputs unchanged

#### Scenario: Export fails
- **WHEN** the encrypted export exits with a non-zero exit code
- **THEN** the collection ends with a failure that names the encrypted export
- **AND** no attachment is requested

### Requirement: Attachments of personal items are downloaded once
The collection SHALL request one download for each attachment that the default or the archived item list shows on a personal item, identified by item ID and attachment ID. An ID pair that both lists show SHALL be requested once. Attachments of organization items SHALL NOT be requested. When an item list is rejected as a document, no attachment SHALL be requested.

#### Scenario: Attachment shown by both lists
- **WHEN** the default and the archived item list both show one personal item with one attachment
- **THEN** that attachment is requested exactly once

#### Scenario: Organization attachment
- **WHEN** the item list shows an attachment on an organization item
- **THEN** that attachment is not requested
- **AND** no download result exists for it

#### Scenario: Unparsable item list
- **WHEN** the archived item list is not valid JSON
- **THEN** no attachment is requested
- **AND** the capture is still passed to the verification

### Requirement: Attachments are handed on one at a time
The collection SHALL hand each successfully downloaded attachment, with its IDs and its file name from the item list, to the receiver before it requests the next one, and SHALL NOT keep the content after the receiver returns. The collection SHALL NOT write attachment content to a file. A failure of the receiver SHALL end the collection.

#### Scenario: Two attachments
- **WHEN** two attachments are downloaded
- **THEN** the receiver gets the first before the second is requested

#### Scenario: Receiver fails
- **WHEN** the receiver refuses an attachment
- **THEN** the collection ends with that failure
- **AND** no further attachment is requested

### Requirement: Download results record hash and size
For each requested attachment, the collection SHALL record exactly one download result. A successful result SHALL carry the SHA-256 and the size of the content. A failed result SHALL carry a download error class and no CLI text.

#### Scenario: Successful download
- **WHEN** an attachment is downloaded with 12 bytes of content
- **THEN** its result carries the SHA-256 of these bytes and the size 12

#### Scenario: Error text with a sentinel
- **WHEN** a download fails and the CLI writes a sentinel value to standard error
- **THEN** the download result and the collection failure do not contain the sentinel

### Requirement: Transient download failures are retried
A download that fails with a timeout, a non-zero exit code without a more specific class, or output of an unexpected form SHALL be retried up to a fixed number of attempts. A download that succeeds on a later attempt SHALL count as successful. When the last attempt fails, the result SHALL be a failed download.

#### Scenario: Second attempt succeeds
- **WHEN** the first attempt of a download times out and the second succeeds
- **THEN** the download result is successful

#### Scenario: Every attempt fails
- **WHEN** every attempt of a download exits with a non-zero exit code
- **THEN** the download result is failed
- **AND** the collection goes on with the next attachment

### Requirement: Premium failure is a failed download
A download that fails because the CLI requires premium SHALL NOT be retried and SHALL be recorded as a failed download. It SHALL NOT lead to a result more favorable than any other failed download.

#### Scenario: Premium required
- **WHEN** a download fails with the premium sentence of the CLI
- **THEN** the download is requested once
- **AND** its result is a failed download
- **AND** the collection goes on with the next attachment

### Requirement: Run-level failures end the collection
A download that fails because the client is not logged in, the vault is locked, the run was cancelled, the CLI could not be started, the CLI did not use the run's data directory, or a file appeared in the working directory SHALL end the collection without a capture. Such a download SHALL NOT be retried.

#### Scenario: Session lost during the downloads
- **WHEN** the second of three downloads fails and the status reports the vault as locked
- **THEN** the collection ends with a failure that names the attachment download
- **AND** the third attachment is not requested

#### Scenario: Cancelled run
- **WHEN** the run is cancelled during a download
- **THEN** the collection ends without a capture
- **AND** the collection starts no further process

### Requirement: Decryption error counts are passed on
The collection SHALL pass on the number of decryption error lines of each of the four documents and of each successful download together with the capture. A count SHALL NOT turn a successful command into a failed one.

#### Scenario: Plaintext export with decryption errors
- **WHEN** the plaintext export exits with code 0 and writes three decryption error lines
- **THEN** the capture carries the count three for the plaintext export

#### Scenario: Download with decryption errors
- **WHEN** an attachment download succeeds and writes three decryption error lines
- **THEN** its download result is successful and carries the count three
