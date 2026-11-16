# Archive Format and Generation Layout

Status: proposed

## Context

A backup must be readable years later, offline, with a separately stored key and without this project's software ([requirements](../product/requirements.md)). Each backup gets a new target. Older backups are never overwritten, even with concurrent runs, and an aborted run must not leave a result that looks successful. Plaintext may exist only in process memory and inside the encryption stream ([architecture](../development/architecture.md)). Backup targets include external drives with FAT or exFAT file systems. Users and scripts will rely on the names a generation gets, and every generation written must stay readable by later versions. Changing these choices later is therefore expensive.

## Decision

**Container.** One generation is one tar stream encrypted with age ([ADR 0003](0003-age.md)) in a single file `vault.tar.age`. Every entry is a regular file with mode 0600, owner and group 0 without names, and the creation time of the generation. No local user name, path or host name enters the archive.

**Layout inside the archive.**

```text
RECOVERY.txt
documents/export.json
documents/export.encrypted.json
documents/items.json
documents/items.archived.json
attachments/<item-id>/<attachment-id>
manifest.json
```

The recovery instructions come first and hold no vault data. The four captured documents are stored byte for byte: the plaintext export, the account-bound encrypted export, the default item list and the archived item list. Each of them is required. The two item lists stay separate documents, because a merged list is a document `bw` never wrote. Attachments are stored by item ID and attachment ID. Their file names appear only in the manifest, inside the encryption.

**Manifest last.** `manifest.json` is the last entry. It holds the format name `bw-takeout-generation` and version 1, the creation time, the source server and account, the tool and `bw` versions, the item and attachment counts, the verification result embedded byte for byte, and size and SHA-256 of every other entry. A reader refuses an unknown format version and ignores unknown additional keys.

**Generation directory.** A generation is a directory `bw-takeout-<YYYYMMDD>T<HHMMSS>Z-<8 hex digits>` in an existing target, with the time in UTC and a random part. It is created with the marker `.partial` and renamed only after the archive is written and flushed. An unverified capture is published with the marker `.unverified`, a failed capture is not published. A directory without `.partial` is complete by construction. No run modifies content that existed before it.

## Alternatives

- **zip instead of tar.** A zip reader needs random access to the directory at the end of the file. A verifier would have to hold the decrypted archive in memory or write it to a plaintext file. tar can be read sequentially from the decryption stream.
- **Manifest first.** The manifest holds the hash of every attachment and the verdict, which are known only after the last attachment. Placing it first forces all attachments into memory at once or a second download.
- **One encrypted file per document and attachment.** Easier partial recovery, but file sizes and counts are visible outside the encryption, and consistency across files needs its own scheme.
- **A flat file per generation with a no-replace rename or a hard link.** Both depend on the file system and are not available on FAT or exFAT. Go's `os.Rename` replaces an existing file on Windows, so it cannot guard a flat file. Creating a directory fails on an existing name on every platform and file system.
- **Markers in a separate status file.** A person listing the target would not see them. A crash between writing the archive and the status file leaves an ambiguous state.

## Consequences

- age and tar alone recover a generation. The recovery test checks this with the age and tar commands.
- Each attachment is held in memory once while it is written, because a tar header states the size before the content. Memory is bounded by the largest attachment.
- Verification reads the whole stream once, with memory bounded by the manifest. Truncation shows only at the end of the stream, so a reader must consume it fully before reporting success.
- Attachment file names, item names and vault content are never visible outside the encryption. The directory name reveals the creation time.
- A crash leaves a `.partial` directory with ciphertext only. It is reported and never removed automatically, because without a lock a dead run cannot be told from a running one.
- The format version and the directory names become a promise. A new format version needs a reader that still opens version 1, and changing the names or markers needs a superseding ADR.
- Format version 1 was redefined once, before any release: the change archived-item-list added `documents/items.archived.json` as a required entry without a new version, because no command wrote a generation yet and the only archives were built inside tests. Archives of the earlier three-document layout are refused. From the first release on, every layout change needs a new format version.
- Directory flushing is not available on Windows, and FAT or exFAT give weak guarantees after a power loss. Offline verification detects the resulting damage but cannot prevent it.

The rules in detail are in the design of the change archive-generation.
