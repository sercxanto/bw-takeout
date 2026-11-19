# Security

The archive contains passkey keys and other highly sensitive data. Anyone who has both an archive and a matching identity can read all of it. Handling of temporary plaintext and leftovers from aborted runs is defined for writing and verifying archives. The handling of the `bw` process and its profile, described below, is built and tested against a stand-in for `bw`. `bw-takeout backup` uses it, and was run by hand against a Vaultwarden server on Linux and macOS, see [testing](../development/testing.md#backup-exercise-of-18-november-2026).

## Key Separation

Archives are encrypted with age to one or more recipient keys ([ADR 0003](../decisions/0003-age.md)). The backing-up system needs only the public recipient keys and holds no secret that opens an archive. Keep the identities, the secret keys, away from the archive and from the backing-up system. Each matching identity alone opens an archive, so encrypting to two recipients whose identities are stored in different places protects against losing one.

The verify command reads the identity only from a file. Error messages about an identity file name the line, never its content.

## Plaintext

Writing and verifying an archive keep plaintext in memory and inside the encryption stream. Neither creates a temporary file. Manual recovery with age and tar writes plaintext to disk, see [recovery](recovery.md).

## Swap, Hibernation and Core Dumps

This program writes no vault content to a file, but the operating system can still put the memory of the process on disk. During a backup the four captured documents, among them the plaintext export with every password, note and passkey key, stay in memory for the whole download phase, because the verification needs them together with the download results. They are overwritten as soon as the capture is parsed. Decoded copies of parts of them stay in memory as text until the Go runtime frees them, and they cannot be overwritten.

While they are in memory, three things can copy them to disk:

- **Swap**: the system can move memory of the process to a swap file or partition.
- **Hibernation**: the system writes all memory to disk when it hibernates.
- **Core dumps**: a crash of the process can write its memory to a core file, depending on the settings of the system.

The same holds for every `bw` process the backup starts: it decrypts the whole vault for an export, the master password at the login, or one attachment.

What a run limits before it starts the first `bw` process:

- **Linux and macOS**: the soft and the hard core file size limit of the process are set to zero and read back. Every `bw` process inherits both limits and cannot raise them. A run whose limit cannot be set, or does not read back as zero, ends in the checks with no generation, before it creates a profile and before it starts `bw`.
- **Linux, in addition**: the `bw-takeout` process marks itself as not dumpable and reads the mark back. The kernel then writes no core for it at all, also not through a crash handler. The mark also means that other processes of the same user can no longer read the memory of the run or trace it, so a debugger cannot attach to a running backup. A run whose mark cannot be set ends in the checks in the same way. Starting a program resets the mark, so it does not reach the `bw` processes, which keep only the core limit.
- **Windows**: nothing is limited yet. Windows Error Reporting can still write a dump of a crashed process, depending on the settings of the system. The report says `Crash dumps: not limited on this platform`, and the run continues.

What stays outside the control of this program: on Linux, the setting `core_pattern` can pipe a core to a crash handler, such as systemd-coredump, apport or abrt. The kernel then does not apply the core limit itself. It passes the limit to the handler, and the handler decides on its own whether it stores the core of a `bw` process. Whether the common handlers honour a limit of zero was not checked on a real distribution, see [testing](../development/testing.md#crash-dump-tests). A `core_pattern` that names a file is subject to the core limit, so no such core is written for `bw-takeout` or a `bw` process. Should a file still appear in the empty working directory of a `bw` process, it ends the run with no generation, and teardown removes it with the profile.

`bw-takeout verify` limits nothing: on every platform a crash of it can write the identity and decrypted archive content to a core file, depending on the settings of the system.

Use encrypted swap or an encrypted disk, such as FileVault on macOS, BitLocker on Windows or LUKS on Linux, on any system that runs a backup or opens a generation. The program does not lock its memory against swap and does nothing about hibernation.

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

Each profile is a directory `profile-<time>-<random>` with a data directory and an empty working directory. The data directory holds the file `bw` keeps: a device ID and the server configuration from the first start, and after a login the access tokens and the encrypted vault cache. At the end of a run the program logs out and removes the whole profile, also when the logout fails. In a manual exercise against Vaultwarden, the data file still held 12 account entries after `bw logout`. Whether they still hold values was not examined, so treat a profile as sensitive until it is removed.

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
