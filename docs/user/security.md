# Security

The archive contains passkey keys and other highly sensitive data. Anyone who has both an archive and a matching identity can read all of it. Handling of temporary plaintext and leftovers from aborted runs is defined for writing and verifying archives. The handling of the `bw` process and its profile, described below, is built and tested against a stand-in for `bw`. The backup run that uses it does not exist yet.

## Key Separation

Archives are encrypted with age to one or more recipient keys ([ADR 0003](../decisions/0003-age.md)). The backing-up system needs only the public recipient keys and holds no secret that opens an archive. Keep the identities, the secret keys, away from the archive and from the backing-up system. Each matching identity alone opens an archive, so encrypting to two recipients whose identities are stored in different places protects against losing one.

The verify command reads the identity only from a file. Error messages about an identity file name the line, never its content.

## Plaintext

Writing and verifying an archive keep plaintext in memory and inside the encryption stream. Neither creates a temporary file. Manual recovery with age and tar writes plaintext to disk, see [recovery](recovery.md).

## Permissions

On Linux and macOS a generation directory is created with mode 0700 and the archive file with mode 0600, so only the owner can read them. Only Linux was tested. Windows access control lists are not set, and FAT or exFAT media have no permissions at all. Protect such targets by other means.

## Session Key

The login prints a session key that unlocks the vault for later `bw` commands. It is read from a pipe and kept in memory. It is given to `bw` processes in their environment, never as a command-line argument, and it is overwritten at logout ([ADR 0006](../decisions/0006-secret-boundary.md)). While a `bw` process runs, other processes of the same user that can read its environment can read the key. Copies of it can stay in the memory of this program until the run ends.

Only a fixed list of environment variables is passed to `bw`. Variables such as `BW_CLEANEXIT` or `NODE_OPTIONS` in your own environment have no effect on the commands this program starts.

## The CLI Profile of a Run

Each run uses its own CLI profile, a new directory that is separate from your own `bw` profile. It is created under the user cache directory with mode 0700 on Linux and macOS:

| System | Location |
|---|---|
| Linux | `$XDG_CACHE_HOME/bw-takeout/profiles`, by default `~/.cache/bw-takeout/profiles` |
| macOS | `~/Library/Caches/bw-takeout/profiles` |
| Windows | `%LocalAppData%\bw-takeout\profiles` |

Each profile is a directory `profile-<time>-<random>` with a data directory and an empty working directory. The data directory holds the file `bw` keeps: a device ID and the server configuration from the first start, and after a login the access tokens and the encrypted vault cache. At the end of a run the program logs out and removes the whole profile, also when the logout fails. What `bw logout` leaves in a profile is known from reading the `bw` code only.

On Windows no access control lists are set for the profile.

## What Is Visible Without a Key

Item names, attachment file names, IDs and all vault content are inside the encryption. The directory name of a generation shows its creation time and whether it is partial or unverified. The size of the archive file is visible.

## No Proof of Origin

Recipient encryption does not authenticate the sender. Anyone who knows a recipient key can create an archive that passes the offline check. A successful check proves that the archive is consistent and readable with your identity, not that this project or your system wrote it. Store archives where others cannot place files.

## Leftovers

An aborted run leaves a directory with the `.partial` marker that holds ciphertext only. It is never removed automatically, because it may belong to a run that is still in progress. Remove it by hand once you know that no run is active.

A run that is killed before its end can also leave its CLI profile behind. Such a profile can hold access tokens and the encrypted vault cache. The next run lists every profile directory of another run with its path and age, and never removes it, for the same reason as above. Once you know that no run is active, delete the listed directory with everything in it, for example on Linux or macOS:

```sh
rm -r -- "<listed path>"
```
