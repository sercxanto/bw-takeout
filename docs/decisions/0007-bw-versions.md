# Handling of bw Versions

Status: proposed

## Context

This project starts the unmodified official `bw` ([ADR 0001](0001-official-cli.md)) and does not ship it. Users install and update `bw` themselves, so a run can meet any version, including one that was never tested here. The project's evidence on `bw` behavior is tied to one version, 2026.10.1, and partly to code reading of its bundle ([code notes](../research/bw-cli-code-notes-2026-11-15.md)). A later version can change a command, a document form or a failure text.

The architecture page expected the adapter to "check version and hash". On 15 November 2026 the maintainer decided otherwise: the version never stops a run and never produces a message for the user. This sets what a user can expect after a `bw` update and how the project relates to its integration boundary, so it is recorded here.

## Decision

- Any `bw` version runs. The client reports the version that `bw --version` prints and the SHA-256 of the executable file after following symbolic links. It does not refuse, warn or change any result because of either value.
- The version goes into the manifest of a generation. Whether the hash goes there too is decided with the backup run.
- The versions this project was tested with are listed for developers in the [integration notes](../development/integrations.md#tested-bw-versions), each with its evidence level per platform. The list is not shown to users.
- Differences in the export fields are reported at run time by a later change. That is the signal a user sees when `bw` changes what it exports.
- What the client relies on is checked on every run from the output itself: the status document is parsed strictly, the session key must have its form, documents must not be empty, and a failure is never taken for a success.

## Alternatives

- **Refuse unknown versions.** Every `bw` update would stop backups until this project releases a new version, and users would be tempted to hold back security updates of `bw` or to bypass the check.
- **Warn on unknown versions.** A warning on every run after each update trains users to ignore it and says nothing about what changed. The check of the export fields gives a more specific signal.
- **Pin a hash.** A hash does not survive any update and differs between the npm layout and the single-file builds per platform. In the npm layout it covers `bw.js` only and not the SDK module file beside it. It would add the cost of refusing without a clear gain.

## Consequences

- A behavior change of `bw` that keeps the document forms and the exit codes is not noticed at run time. The tests with the real `bw` and the review of a new version in the dev container are the means to find it.
- A tampered `bw` is not detected by this project. The hash is information for the manifest and for a developer, not a defence. Protecting the installed `bw` stays with the user's system.
- No release of this project is needed for a new `bw` version, as long as the forms the client checks stay the same.
- The tested versions are a developer record and need to be kept current when the dev container pin changes.

The rules in detail are in the design of the change bw-process-adapter, decision D16.
