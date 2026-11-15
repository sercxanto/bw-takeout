# Limitations

A successful CLI export can deliver decryptable partial inventories while items or passkeys are missing. Missing organization permissions and stale local state limit the view further. An independent reference inventory is often unknown. See [evidence levels](../research/research-summary.md).

## What a Verdict Can and Cannot Detect

Every capture is judged failed, unverified or verified ([ADR 0004](../decisions/0004-completeness-verdict.md)). The verification is planned and not yet part of a usable tool. Its rules are tested with synthetic data only. No live capture backs them yet.

It detects:

- an item that is in the encrypted export but missing from the plaintext export, and the reverse,
- a passkey missing from an item whose passkey count the encrypted export shows,
- a passkey with a missing field, an empty or unparsable key, or a counter that is not a non-negative number,
- an attachment that failed to download, has no download result, or has a result that the item list does not explain,
- a personal item that the item list shows and the plaintext export lacks.

It cannot detect:

- **Missing passkeys in blob-format items.** The encrypted export seals the content of such items, so their passkey count is not visible. A blob login item is reported as unverified, never as "no passkeys".
- **A swapped passkey or a wrong key without a reference.** If a passkey is replaced by another valid passkey, or a key is well-formed but wrong, the counts and the structure still match. Every result without a reference inventory lists credential identity and attachment content as not checked.
- **A missing attachment entry.** The decrypted item list is the only source of attachment metadata. An attachment that is missing from an otherwise intact list entry stays undetected. Every result without a reference inventory lists this limit.
- **Items the item list does not show.** Such items are reported as unverified, because their attachments are unknown. A default listing leaves out archived items, so the listing of archived items has to be included.
- **Completeness against the server.** "Verified" covers the personal items in the local snapshot that the CLI exported during the capture. An omission in a step that both exports share, and items that the CLI leaves out of every document, stay invisible. Every result states this scope and lists it as a limit.

Organization items are not checked. They are counted per organization and reported as a warning.

## What Offline Verification Shows

`bw-takeout verify` shows that:

- the identity file holds an identity that opens the archive,
- the archive is complete and unaltered: a truncated file or a changed byte fails the check,
- every entry has a name the format defines and matches the size and SHA-256 in the manifest, and no entry is missing or added,
- the manifest has a format version this program knows.

It does not show:

- **Completeness of the capture.** The stored verdict was computed when the backup was made. The check reports it but does not compute it again from the archived documents, and it carries all limits of that verdict described above.
- **Origin.** Anyone who knows a recipient key can create an archive that passes. See [security](security.md).
- **That the documents import elsewhere.** The plaintext export is stored as `bw` wrote it. Whether another program imports it is not tested.
- **The state of other generations.** Each generation is checked on its own.

## Storage Guarantees

A generation directory loses its `.partial` marker only after its archive file was flushed to storage. Where the platform supports it, the generation directory and the target directory are flushed as well. Some guarantees depend on the platform and the file system:

- **Windows** cannot flush a directory. After a power loss, a rename that the program reported as done can be lost, and the generation can reappear under its `.partial` name. The archive file itself was flushed before the rename.
- **FAT and exFAT media** keep no journal. After a power loss or when a drive is pulled without ejecting, a directory entry or the end of a file can be damaged even after a successful flush. These file systems also have no POSIX permissions, so the owner-only modes are not applied there.
- **Network shares** are untested. Their flush and rename semantics depend on the server.

Offline verification detects a damaged or truncated archive but cannot repair it. Keep more than one generation, and on removable media eject the drive before unplugging it.

Directory rename, flushing and permissions were tested on Linux only. macOS and Windows have not been tested yet.
