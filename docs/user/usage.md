# Usage

`bw-takeout backup` backs up one account interactively. `bw-takeout verify` checks a backup offline. The backup command is tested against a stand-in for `bw`, and was run by hand against a Vaultwarden server on Linux and macOS on 18 November 2026, see [testing](../development/testing.md#backup-exercise-of-18-november-2026). Bitwarden Cloud and Windows are untested.

## Making a Backup

```sh
bw-takeout backup --server <https-url> --target <dir> --recipients-file <file> [--bw <path>] [--ca <file>] [--organization <id>]...
```

One run backs up one account. You type the login, the master password and the two-step code at the terminal, where `bw` prompts for them. Standard input and standard error must be a terminal. Standard output can be redirected to a file. The flags come before any argument, and the command takes no argument.

| Flag | Meaning |
|---|---|
| `--server <https-url>` | Required. The HTTPS address of the server, such as `https://vault.example.org`. It must have a host and no user information. It is required so that a fresh profile never falls back to the default cloud server unnoticed. |
| `--target <dir>` | Required. An existing directory that receives the new generation. Existing content is never changed. |
| `--recipients-file <file>` | Required. The age recipients the archive is encrypted to. |
| `--bw <path>` | Optional. The `bw` executable. Without it, `bw` is looked up on the program search path. A relative path is taken relative to the working directory. |
| `--ca <file>` | Optional. An additional certificate authority file, for a server with a certificate of your own authority. |
| `--organization <id>` | Optional and repeatable. The ID of an organization to back up as well, in the form of a UUID. The account must be its owner or admin, see [Organizations](#organizations). The ID is compared without regard to case, and the same ID given twice is a usage error. |

Before it starts `bw`, the command first limits crash dumps: on Linux and macOS it sets the core file size limit of its process to zero, which every `bw` process inherits, and on Linux it also marks its own process as not dumpable, see [security](security.md#swap-hibernation-and-core-dumps). On Linux and macOS a run whose limit or mark cannot be set ends here with `Failed:      checks:` and the part that failed, and exit status 1. On Windows crash dumps are not limited, and the run continues. Then the command checks that the target is an existing directory, that the recipients file holds at least one recipient, that `bw` can be found and is a regular file, and that a given CA file can be read. A mistake then costs no login and no two-step code.

### Organizations

Without `--organization`, a run backs up the personal vault only. Items of organizations that `bw list items` shows still end up in `documents/items.json`, because that list is stored as `bw` wrote it, but they are not checked and their attachments are not downloaded.

With `--organization <id>`, the run also backs up the whole organization with that ID. Give the flag once per organization. The ID is shown in the address bar of the web vault when the organization is open, or by `bw list organizations`. For each chosen organization:

- After the sync, the run reads `bw list organizations`. The organization must be listed with a confirmed membership and the role owner or admin. Otherwise the run ends with no generation before any document is requested, and the report names the organization ID and the reason. A user, a manager or a custom role is refused, also when a custom role has the export permission, see [limitations](limitations.md#organizations).
- The run requests the plaintext and the encrypted organization export with `bw export --organizationid`. Unlike the personal exports, each call fetches the organization's items from the server. A failure of either export ends the run with no generation.
- The attachments of the organization's items that the item lists show are downloaded like those of personal items.
- The two exports are checked against each other and against the item lists, and each item must have the same revision date in all of them. A difference fails the capture with the reason `organization_changed_during_capture`: the organization was edited during the run. Run the backup again.

Each export is recorded as an event in the organization's event log, visible to its admins, see [security](security.md#organizations).

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

Progress and report go to standard output. The prompts of `bw` and the interrupt messages go to standard error. The report holds no vault content: no item names, user names, passwords, notes, field values, keys, attachment file names, organization names, session key, and no text `bw` wrote. Characters that are not printable are replaced with `?`.

In this order, it shows:

- the `bw` executable with its version and SHA-256, the server, the target, the number of recipients and, when organizations are chosen, a line `Organizations:` with their IDs in the order given,
- the crash dump protection of the run: `Crash dumps: core limit 0, process not dumpable` on Linux, `Crash dumps: core limit 0` on macOS, and `Crash dumps: not limited on this platform` on Windows,
- leftover profiles of other runs and leftover partial generations in the target, each with its path and age, or `none`. They are never changed or removed, see [security](security.md#leftovers),
- after the sync, the server the status names, or the one given with `--server` when the status names none, and the account address and ID,
- the number of planned attachments and, once all downloads have ended, one line per download in order. A failed download names its item ID, attachment ID and error class. While the downloads run, no line appears, and after an interrupt or a failure during the downloads no attachment line appears,
- the verdict with its scope statement, its standing limits, and every finding and warning, one per line with severity, reason code and IDs. A finding about an organization names it with `organization=<id>`. A warning about an export key unknown to this project names its key path with `field=`, for example `field=items[].login.newKey`, and a warning about an unknown item type its number with `item_type=` ([key names in the report](security.md#key-names-in-the-report)). This is shown for every verdict, also when no generation is published,
- the path of the published generation, with a warning when the target directory could not be flushed to storage afterwards, or that none was published. When a partial generation of this run could not be removed, the report says that it stays in the target,
- what ended the run: a failure with its phase, its `bw` operation and class, for an organization command its organization ID, for a refused organization its ID and the reason, or the phase in which it was interrupted,
- the result of the logout with its duration, and whether the profile was removed. A profile that was not removed is named by its path.

For a failed verdict the report is the only record, because no generation is published.

### Exit Status

| Exit status | Meaning |
|---:|---|
| 0 | Generation published, capture verified, teardown complete |
| 3 | Generation published, capture unverified, teardown complete |
| 4 | Generation published, but the logout did not succeed or the profile was not removed |
| 1 | No generation: a failed check, login, sync, organization check, export or download, a failed verdict or an interrupt |
| 2 | Usage error: a missing required flag, an unknown flag, an extra argument, a server address that is not HTTPS, or an organization ID that is not a UUID or is given twice |

Teardown is complete when the logout succeeded, or no login was started, and the profile was removed. 0, 1, 2 and 3 mean the same as for `verify`. See [limitations](limitations.md#interrupting-a-backup) for interrupts.

To check a generation later, on this or another system, use `bw-takeout verify` below.

## Checking a Generation Offline

```sh
bw-takeout verify --identity <identity-file> <generation-directory>
```

The command checks one backup generation with an age identity file. It needs no network access and no `bw`, starts no other program and writes no file. The identity can only be given as a file, never as an argument or environment variable. The flag comes before the directory.

It decrypts the archive, reads it to its end, compares the size and SHA-256 of every entry with the manifest and rejects entries the format does not define. On success it prints the generation name, creation time, source server and account, the number of items and attachments, one line per chosen organization with its ID and number of items, and the stored verdict with the number of findings and warnings. It prints no vault content, no attachment file names and no organization names. A generation written before organizations were supported has no organization list in its manifest and is refused. Failures name the affected entry by its path, which holds only IDs.

| Exit status | Meaning |
|---:|---|
| 0 | Intact generation with a verified capture |
| 3 | Intact generation with an unverified capture |
| 1 | Verification failed: damaged, truncated or inconsistent archive, no matching identity, unreadable identity file, unsupported format version, or an incomplete generation (`.partial` marker or no archive file) |
| 2 | Usage error |

The stored verdict was computed when the backup was made, by the version named in the manifest. The command does not compute it again. See [recovery](recovery.md) for opening a generation without this project and [limitations](limitations.md) for what the check shows.
