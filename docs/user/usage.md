# Usage

`bw-takeout backup` backs up one account interactively. `bw-takeout verify` checks a backup offline. The backup command is tested against a stand-in for `bw`, and was run by hand against a Vaultwarden server on Linux and macOS on 18 November 2026, see [testing](../development/testing.md#backup-exercise-of-18-november-2026). Bitwarden Cloud and Windows are untested.

## Making a Backup

```sh
bw-takeout backup --server <https-url> --target <dir> --recipients-file <file> [--bw <path>] [--ca <file>]
```

One run backs up one account. You type the login, the master password and the two-step code at the terminal, where `bw` prompts for them. Standard input and standard error must be a terminal. Standard output can be redirected to a file. The flags come before any argument, and the command takes no argument.

| Flag | Meaning |
|---|---|
| `--server <https-url>` | Required. The HTTPS address of the server, such as `https://vault.example.org`. It must have a host and no user information. It is required so that a fresh profile never falls back to the default cloud server unnoticed. |
| `--target <dir>` | Required. An existing directory that receives the new generation. Existing content is never changed. |
| `--recipients-file <file>` | Required. The age recipients the archive is encrypted to. |
| `--bw <path>` | Optional. The `bw` executable. Without it, `bw` is looked up on the program search path. A relative path is taken relative to the working directory. |
| `--ca <file>` | Optional. An additional certificate authority file, for a server with a certificate of your own authority. |

Before it starts `bw`, the command first limits crash dumps: on Linux and macOS it sets the core file size limit of its process to zero, which every `bw` process inherits, and on Linux it also marks its own process as not dumpable, see [security](security.md#swap-hibernation-and-core-dumps). On Linux and macOS a run whose limit or mark cannot be set ends here with `Failed:      checks:` and the part that failed, and exit status 1. On Windows crash dumps are not limited, and the run continues. Then the command checks that the target is an existing directory, that the recipients file holds at least one recipient, that `bw` can be found and is a regular file, and that a given CA file can be read. A mistake then costs no login and no two-step code.

### Recipients File

The recipients file holds one native age X25519 recipient per line, in the form `age1...`. Empty lines and lines starting with `#` are skipped. A line that is neither is refused with its line number, never its content. The file holds public keys only. Keep the matching identities away from the backing-up system, see [security](security.md).

```text
# Backup key at home
age1<public key of your first identity>
# Backup key in the safe deposit box
age1<public key of your second identity>
```

Replace both placeholder lines with your own recipients: `age-keygen -y <identity-file>` prints the recipient of an identity. The placeholders do not parse, so a copied example is refused instead of encrypting to a key you do not hold. Check the number of recipients in the report.

Each identity alone opens the archive.

### What the Report Shows

Progress and report go to standard output. The prompts of `bw` and the interrupt messages go to standard error. The report holds no vault content: no item names, user names, passwords, notes, field values, keys, attachment file names, session key, and no text `bw` wrote. Characters that are not printable are replaced with `?`.

In this order, it shows:

- the `bw` executable with its version and SHA-256, the server, the target and the number of recipients,
- the crash dump protection of the run: `Crash dumps: core limit 0, process not dumpable` on Linux, `Crash dumps: core limit 0` on macOS, and `Crash dumps: not limited on this platform` on Windows,
- leftover profiles of other runs and leftover partial generations in the target, each with its path and age, or `none`. They are never changed or removed, see [security](security.md#leftovers),
- after the sync, the server the status names, or the one given with `--server` when the status names none, and the account address and ID,
- the number of planned attachments and, once all downloads have ended, one line per download in order. A failed download names its item ID, attachment ID and error class. While the downloads run, no line appears, and after an interrupt or a failure during the downloads no attachment line appears,
- the verdict with its scope statement, its standing limits, and every finding and warning, one per line with severity, reason code and IDs. This is shown for every verdict, also when no generation is published,
- the path of the published generation, with a warning when the target directory could not be flushed to storage afterwards, or that none was published. When a partial generation of this run could not be removed, the report says that it stays in the target,
- what ended the run: a failure with its phase, its `bw` operation and class, or the phase in which it was interrupted,
- the result of the logout with its duration, and whether the profile was removed. A profile that was not removed is named by its path.

For a failed verdict the report is the only record, because no generation is published.

### Exit Status

| Exit status | Meaning |
|---:|---|
| 0 | Generation published, capture verified, teardown complete |
| 3 | Generation published, capture unverified, teardown complete |
| 4 | Generation published, but the logout did not succeed or the profile was not removed |
| 1 | No generation: a failed check, login, sync or download, a failed verdict or an interrupt |
| 2 | Usage error: a missing required flag, an unknown flag, an extra argument or a server address that is not HTTPS |

Teardown is complete when the logout succeeded, or no login was started, and the profile was removed. 0, 1, 2 and 3 mean the same as for `verify`. See [limitations](limitations.md#interrupting-a-backup) for interrupts.

To check a generation later, on this or another system, use `bw-takeout verify` below.

## Checking a Generation Offline

```sh
bw-takeout verify --identity <identity-file> <generation-directory>
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
