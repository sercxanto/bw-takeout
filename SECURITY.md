# Security Policy

bw-takeout is meant to handle the most sensitive data a user has: a full password vault including passkey private keys. Reports about its security are welcome, also while the project is unfinished.

## Supported Versions

There is no release yet. Fixes go to the `main` branch only. Until a first release, no version is supported for use with a real vault, see the [README](README.md).

## Reporting a Vulnerability

Report privately through GitHub: open the **Security** tab of this repository and choose **Report a vulnerability**, or use [this form](https://github.com/sercxanto/bw-takeout/security/advisories/new). Do not open a public issue, pull request or discussion for a vulnerability.

Please include:

- what is affected: a command, a package, the archive format, a workflow or a document,
- the steps or a test that shows the problem, and the platform it ran on,
- what an attacker gains, and what access the attacker needs.

**Use synthetic data only.** Never put real vault content, credentials, session keys, plaintext exports, archives of a real vault, age identities or CLI profiles into a report. A test with a synthetic vault, such as the fixtures under `internal/` or a local Vaultwarden, is enough.

## What Counts as a Vulnerability

In scope, among others:

- vault content, the session key or an identity reaching a log, an error message, a temporary file, a command-line argument or any other output of this project,
- an archive that can be read without a matching identity, or a weakness in how archives are written, verified or published,
- a capture or a generation reported as complete or verified although items, passkeys or attachments are missing,
- text or behavior of `bw` that this project forwards or trusts against the rules of [ADR 0006](docs/decisions/0006-secret-boundary.md),
- file permissions of generations and CLI profiles that are weaker than the [security page](docs/user/security.md) states,
- a weakness in the CI workflows or the pinned dependencies of this repository.

Out of scope:

- vulnerabilities in Bitwarden, the official `bw` CLI, Vaultwarden, age or Go itself; report them to those projects,
- attacks that need control of the backing-up system or of the user account running bw-takeout, beyond what the [security page](docs/user/security.md) already describes as a limit,
- the limits stated in the [security page](docs/user/security.md) and the [limitations](docs/user/limitations.md), such as the missing proof of origin of an archive.

If you are unsure whether something is in scope, report it privately anyway.

## What Happens Next

bw-takeout is maintained by one person in their spare time. Reports are answered on a best-effort basis, without a guaranteed response time and without a bug bounty. Once a fix is on `main`, the report is published as a GitHub security advisory, with credit to the reporter if they wish.

bw-takeout is an independent project. It is not affiliated with, endorsed by or sponsored by Bitwarden, Inc.
