# Usage

An interactive backup of one account with TOTP is planned. Its command, progress display and result status will be added after the first verified implementation.

## Checking a Generation Offline

```sh
bitwarden-backup verify --identity <identity-file> <generation-directory>
```

The command checks one backup generation with an age identity file. It needs no network access and no `bw`, starts no other program and writes no file. The identity can only be given as a file, never as an argument or environment variable. The flag comes before the directory.

It decrypts the archive, reads it to its end, compares the size and SHA-256 of every entry with the manifest and rejects entries the format does not define. On success it prints the generation name, creation time, source server and account, the number of items and attachments, and the stored verdict with the number of findings and warnings. It prints no vault content and no attachment file names. Failures name the affected entry by its path, which holds only IDs.

| Exit status | Meaning |
|---:|---|
| 0 | Intact generation with a verified capture |
| 3 | Intact generation with an unverified capture |
| 1 | Verification failed: damaged, truncated or inconsistent archive, no matching identity, unreadable identity file, unsupported format version, or an incomplete generation (`.partial` marker or no archive file) |
| 2 | Usage error |

The stored verdict was computed when the backup was made, by the version named in the manifest. The command does not compute it again. See [recovery](recovery.md) for opening a generation without this project and [limitations](limitations.md) for what the check shows.
