# Security

The archive contains passkey keys and other highly sensitive data. Anyone who has both an archive and a matching identity can read all of it. Handling of temporary plaintext and leftovers from aborted runs is defined for writing and verifying archives. The backup run itself does not exist yet.

## Key Separation

Archives are encrypted with age to one or more recipient keys ([ADR 0003](../decisions/0003-age.md)). The backing-up system needs only the public recipient keys and holds no secret that opens an archive. Keep the identities, the secret keys, away from the archive and from the backing-up system. Each matching identity alone opens an archive, so encrypting to two recipients whose identities are stored in different places protects against losing one.

The verify command reads the identity only from a file. Error messages about an identity file name the line, never its content.

## Plaintext

Writing and verifying an archive keep plaintext in memory and inside the encryption stream. Neither creates a temporary file. Manual recovery with age and tar writes plaintext to disk, see [recovery](recovery.md).

## Permissions

On Linux and macOS a generation directory is created with mode 0700 and the archive file with mode 0600, so only the owner can read them. Only Linux was tested. Windows access control lists are not set, and FAT or exFAT media have no permissions at all. Protect such targets by other means.

## What Is Visible Without a Key

Item names, attachment file names, IDs and all vault content are inside the encryption. The directory name of a generation shows its creation time and whether it is partial or unverified. The size of the archive file is visible.

## No Proof of Origin

Recipient encryption does not authenticate the sender. Anyone who knows a recipient key can create an archive that passes the offline check. A successful check proves that the archive is consistent and readable with your identity, not that this project or your system wrote it. Store archives where others cannot place files.

## Leftovers

An aborted run leaves a directory with the `.partial` marker that holds ciphertext only. It is never removed automatically, because it may belong to a run that is still in progress. Remove it by hand once you know that no run is active.
