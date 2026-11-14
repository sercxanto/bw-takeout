# Design

## Context

See [proposal.md](proposal.md) for motivation. The repository holds the packages `internal/capture` and `internal/verify` and no command yet. The component model in [architecture.md](../../../docs/development/architecture.md) names `manifest`, `archive` and `publish` as planned, with two rules that bind this change: plaintext exists only in process memory and inside the encryption stream, and only one component touches the target directory.

Decisions the maintainer took on 14 November 2026, before this proposal:

1. Encrypt to age recipient keys. A passphrase mode may follow later.
2. Use tar as the container.
3. Publish one directory per generation.
4. Publish an unverified capture with a visible marker and a non-zero exit status. Do not publish a failed capture.

Observed in the dev container on 14 November 2026, in a scratch module outside the repository:

- `filippo.io/age` v1.2.1 encrypts to an X25519 recipient and decrypts as a sequential stream. A truncated ciphertext opens without error and then fails while reading, with an authentication error on the affected chunk. A reader must therefore consume the stream to its end before reporting success.
- `go mod tidy` fails for the user `vscode` because `/go/pkg` belongs to root. The Dockerfile changes the owner of `/go` only, not of its content. With `GOPATH` pointed at a writable directory the same commands succeed.
- age v1.3.2 exists and adds hybrid post-quantum recipients and a random-access decrypt function. Neither was tested.

## Goals / Non-Goals

**Goals:**

- A generation that the standard age and tar commands can open without this project.
- Writing with memory bounded by the largest single attachment, and verifying with memory bounded by the manifest.
- Publishing built only from file system operations that behave the same on Linux, macOS and Windows.
- Tests that run offline in the dev container with synthetic captures.

**Non-Goals:**

- Any `bw` invocation, login, sync or attachment download. A backup command comes with the later change `interactive-backup-run`.
- Passphrase encryption, plugin recipients, SSH keys as recipients and passphrase-protected identity files.
- Extracting an archive to plaintext files. The user can do this with age and tar, as the recovery page describes.
- Retention, deletion of old generations and cleanup of leftovers.
- Recomputing the verdict from the archived documents during offline verification.
- A file format for the reference inventory.
- Setting Windows access control lists.

## Decisions

### D1: Three packages and one command

- `internal/manifest`: manifest types, serialization, format version.
- `internal/archive`: writer and reader for the encrypted tar stream. It works on an `io.Writer` and an `io.Reader` and never touches a path.
- `internal/publish`: the only code that touches the target directory.
- `cmd/bitwarden-backup`: the command with the single subcommand `verify`.

The verdict rule of D9 lives in `publish`, so no caller can publish a failed capture by mistake.

*Alternative considered:* one package for format and publishing. Rejected because the format must be testable without a file system and the publishing rules without encryption.

### D2: Layout inside the archive

```text
RECOVERY.txt
documents/export.json
documents/export.encrypted.json
documents/items.json
attachments/<item-id>/<attachment-id>
manifest.json
```

The three documents are the plaintext export, the account-bound encrypted export and the item list, byte for byte as captured. The plaintext export is the payload for recovery and conversion. The other two are kept as evidence for the embedded verdict and so that later tool versions can re-examine a generation. The encrypted export cannot be read without the account key. That is accepted.

`RECOVERY.txt` comes first so that a person who lists the archive sees it first. `manifest.json` comes last, see D4.

Tar entries are regular files with mode 0600, owner and group 0, empty owner names and the creation time of the generation as modification time. No local user name, path or host name enters the archive.

### D3: Entry names from fixed names and validated IDs

Item IDs and attachment IDs come from CLI output and are used in entry names. An item ID must have the UUID form of 8-4-4-4-12 hexadecimal digits. An attachment ID must consist of 1 to 64 ASCII letters and digits. Anything else aborts the writing. Neither form admits a path separator, a dot or any other character with a meaning in a path.

The two rules differ because the evidence differs. The live probe of 14 November 2026 did not record the form of either ID. The synthetic fixture uses UUIDs for items and 32 alphanumeric characters for attachments. From the upstream server sources, not checked in this repository, both servers use UUIDs for items, the Bitwarden server generates attachment IDs of 32 lowercase alphanumeric characters and Vaultwarden of 20 hexadecimal digits. The alphanumeric rule covers both without depending on either. The maintainer decided this on 14 November 2026.

*Alternative considered:* escaping unusual IDs. Rejected because an ID outside these forms means an unknown server behavior, and refusing is the safe answer.

*Alternative considered:* the UUID form for attachment IDs too. Rejected because it refuses the fixture and, by the upstream sources, every real capture.

### D4: Manifest last

The manifest lists the size and SHA-256 of every other entry and embeds the verification result. Both are known only after all attachments have passed through the writer. With the manifest last, the writer can stream each attachment into the encrypted archive as soon as it has it and needs to keep only hashes. If the verdict then turns out failed, the partial generation is discarded, and only ciphertext ever reached the disk.

The reader hashes entries as they pass, keeps only the hashes, and compares them when the manifest arrives. It needs no second pass and no random access.

*Alternative considered:* manifest first. Rejected because it forces all attachments into memory at once or a second download.

### D5: Manifest content

JSON with these top-level keys:

| Key | Content |
|---|---|
| `format`, `format_version` | The fixed string `bitwarden-backup-generation` and the number 1 |
| `created` | UTC time in RFC 3339 |
| `source` | `server_url`, `account_email`, `account_id`, supplied by the caller |
| `tool` | `name` and `version` of this project |
| `bw` | `version`, supplied by the caller, empty in this change |
| `counts` | Number of items in the plaintext export and number of attachments |
| `verification` | The serialized result of `internal/verify`, embedded as raw JSON without re-encoding |
| `entries` | One object per entry except the manifest: `path`, `kind`, `size`, `sha256`, and for attachments `item_id`, `attachment_id` and `file_name` |

Attachment file names appear here and nowhere else. They are inside the encryption. An unknown `format_version` makes the reader fail. Unknown additional keys are ignored, so later versions can add information without breaking older readers.

### D6: Tar container

Tar was chosen over zip because it can be read sequentially from the decryption stream. A zip reader needs random access to the directory at the end of the file, which would force the verifier to hold the whole decrypted archive in memory or in a plaintext file.

The price is that a tar header states the size before the content, so each attachment is held in memory once while it is written. That is bounded by the largest attachment.

The Go standard library writes the tar stream. Names are short and ASCII, so no extension beyond what the library chooses by itself is required. The integration test of D12 proves that the system tar reads the result.

### D7: age with X25519 recipients

Encryption uses `filippo.io/age` v1.2.1 in binary form, the same version as the age CLI in the dev container. The library is the reference implementation, so no external process is needed and the stream is encrypted in-process.

Recipients are native X25519 recipient strings. At least one is required. Any other recipient form is refused, and error messages name the position of a rejected recipient, not its value. Identities for verification come from an age identity file with unencrypted X25519 identities.

A passphrase mode can be added later. It changes neither container nor manifest. The age format allows a passphrase only as the sole recipient of a file, so a generation will be encrypted either to a passphrase or to recipient keys.

*Consequence:* the backup side holds no secret that opens the archive, so it cannot test decryption after writing. The offline verification with the identity is a separate act.

*Alternative considered:* upgrading to age 1.3 for hybrid recipients in this change. Deferred. It would change the qualified tool version together with a feature. The archive format stays the same, so the upgrade is additive later.

### D8: Generation directory and markers

A generation is a directory in the target:

```text
<target>/bitwarden-backup-<YYYYMMDD>T<HHMMSS>Z-<8 hex digits>[.partial|.unverified]/
    vault.tar.age
```

Steps:

1. Check that the target exists and is a directory.
2. Create `<name>.partial` with `mkdir` and mode 0700. `mkdir` fails if the name exists, on every platform and file system. On failure because the name exists, draw a new random part, for a bounded number of attempts.
3. Create `vault.tar.age` inside with exclusive creation and mode 0600, and write the encrypted stream.
4. Flush the file to storage and close it. Flush the directory where the platform supports it.
5. Check that the final name does not exist, then rename the directory to `<name>` or `<name>.unverified`. Flush the target directory where supported.

The marker is on the directory because that is what a person sees when listing the target. A directory without `.partial` is complete by construction.

No run ever creates a final name directly, and each run owns its partial name exclusively, so two runs cannot aim at the same final name. If a third party created the final name in between, the rename fails on Windows and on a non-empty directory elsewhere. The run then ends with an error and leaves its partial directory. No existing backup is replaced in any case.

*Alternatives considered:* hard links or a no-replace rename of a flat file. Both depend on the file system and are not available on FAT or exFAT media, which are likely backup targets. Go's `os.Rename` replaces an existing file on Windows, so it cannot guard a flat file.

Clock and random source are injectable so that tests can force collisions.

### D9: The verdict decides publication

`publish` takes the verdict at commit time. Verified leads to the plain name, unverified to the `.unverified` marker, failed to removal of the partial directory. An abort for any other reason also leaves or removes only the run's own partial directory.

Partial directories of earlier runs are listed with name and age and never touched. Without a lock there is no safe way to tell a dead run from a running one.

### D10: The verify command

```text
bitwarden-backup verify --identity <file> <generation-directory>
```

It opens `vault.tar.age`, decrypts with the identities from the file, reads the tar stream once and applies the checks of the `offline-verification` spec. It reads the stream to its end before reporting success, because truncation shows only there.

Exit status: 0 for an intact generation with a verified capture, 3 for an intact generation with an unverified capture, 1 for a failed verification, 2 for a usage error. The value 3 keeps unverified apart from both success and failure for scripts.

The output shows the generation name, creation time, source server and account, counts, the stored verdict and the number of findings and warnings. It names entries by their path, which contains IDs. It never prints content from the documents or file names from the manifest.

The identity path is the only way to supply an identity. No identity is accepted as argument or environment variable.

The command does not import the Go network package. A task checks this on the dependency list.

### D11: No plaintext files

The writer receives documents and attachments as byte slices and writes into the encryption stream. The reader hashes entries as they pass. Neither creates temporary files. Tests point the temporary directory of the process at an empty directory and assert that it stays empty.

One test deliberately writes synthetic plaintext to a test directory: the standard-tool recovery test of D12. It uses only generated data.

### D12: Tests and synthetic data

- Captures come from the existing helpers in `internal/capture/capturetest`. Their sentinel values serve the redaction tests of this change.
- Age key pairs are generated at test time. No key is committed.
- A recovery test decrypts with the `age` command and unpacks with the `tar` command from the search path and compares bytes. It is skipped with a visible message where the commands are missing.
- The kill test starts a helper process from the test binary, lets it write part of an archive and kills it.
- The concurrency test runs eight publishers with the same clock value and the same first random value.
- An opt-in test leaves a synthetic generation at a path given by environment variables, encrypted to a given recipient. It serves the manual offline exercise on a second system that the product requirements ask for. It does nothing without the variables.

### D13: Dev container repair and dependency

The Dockerfile makes the content of `/go` writable for the container user, by changing the owner recursively after the tools are installed. The running container is not affected until it is rebuilt, which only the maintainer can do. Until then the implementation session sets `GOPATH` to a directory under the ignored `.local/` for its Go commands. This workaround is temporary and is not written into the repository's instructions.

`go.mod` gains `filippo.io/age v1.2.1` with its indirect dependencies, and `go.sum` is committed. The integration notes record that the library and the CLI are pinned to the same version and updated together.

### D14: ADRs

A new proposed ADR `0005` records the archive format and the generation layout: D2, D4, D5, D6 and D8. Old generations must stay readable, and users and scripts will rely on the directory names and markers, so this is hard to reverse and meets the ADR criteria.

The proposed ADR 0003 on age is updated: the recipient model is settled, key loss is addressed by multiple recipients, and offline recovery is tested. It gains two open points, the missing sender authentication and hybrid recipients in age 1.3. It stays proposed.

D10's exit codes, the command name and the test design are change-specific and stay here.

## Risks / Trade-offs

- [Only Linux arm64 can be run in the dev container] → The code uses no platform-specific call except the directory flush, which is skipped where unsupported. A task cross-compiles for macOS and Windows. Running the tests there is a manual step for the maintainer and is recorded as not run until done.
- [Directory rename can fail on Windows while a scanner holds a file inside] → Retry a few times with a short pause, then fail and leave the partial directory. This is the safe direction.
- [Directory flush does not exist on Windows, and FAT media give weak guarantees after power loss] → Documented as a limit. The offline verification detects a damaged archive.
- [An attachment is held in memory once] → Bounded by the largest attachment. Documented, with a size limit as a possible later addition.
- [Recipient encryption does not prove who wrote an archive] → Stated in the security page, the limitations and ADR 0003. Offline verification proves consistency and readability, not origin.
- [A partial generation can stay behind after a crash] → It holds only ciphertext. It is reported at the next listing and never mistaken for a backup.
- [The stored verdict was computed by the tool version that wrote the generation] → The manifest records that version. Recomputing is a non-goal here.
- [Standard-tool recovery writes plaintext to disk] → That is the user's decision at recovery time. The recovery page says so and recommends an encrypted or temporary volume.

## Migration Plan

No migration. No generation exists yet. The dev container repair takes effect when the maintainer rebuilds the container.

## Open Questions

- Should the generation name later carry an account label once several accounts are supported? The pattern leaves room in front of the timestamp.
- Is an upper limit for a single attachment needed, and what should happen above it?
- Should `verify` also accept a path to the archive file instead of the directory?
