# bw-takeout

A small CLI is planned that backs up a vault from Bitwarden Cloud or Vaultwarden locally in encrypted form, so it can be read offline later. Passkey key material and file attachments are part of the backup scope.

> [!WARNING]
> **Work in progress, not ready for use.** There is no backup command yet, and commands, formats and behavior may change without notice before a first release. Do not use bw-takeout with a real vault and do not rely on it as a backup.

**Project status:** The development environment and research are in place. The completeness verdict, the encrypted archive format and publishing into a target directory are implemented and tested with synthetic data. The command `bw-takeout verify` checks a backup generation offline. Starting the official `bw` CLI, the interactive login and the per-run CLI profile are implemented, tested against a fake `bw` and exercised by hand against Vaultwarden on Linux and macOS. There is no backup command and no installable application yet.

- [User guide and documentation overview](docs/README.md)
- [Limits of the completeness check](docs/user/limitations.md)
- [Open the development environment and start Explore](docs/development/getting-started.md)

The project is licensed under the MIT license, see [LICENSE](LICENSE).

bw-takeout is an independent project. It is not affiliated with, endorsed by or sponsored by Bitwarden, Inc. Bitwarden is a trademark of Bitwarden, Inc.
