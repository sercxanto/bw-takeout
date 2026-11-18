# bw-takeout

A small CLI that backs up a vault from Bitwarden Cloud or Vaultwarden locally in encrypted form, so it can be read offline later. It is a first prototype. Passkey key material and file attachments are part of the backup scope.

> [!WARNING]
> **Work in progress, not ready for use.** The backup command is a first prototype, tested against a stand-in for `bw` and run by hand against Vaultwarden only. Commands, formats and behavior may change without notice before a first release. Do not use bw-takeout with a real vault and do not rely on it as a backup.

**Project status:** The development environment and research are in place. The completeness verdict, the encrypted archive format and publishing into a target directory are implemented and tested with synthetic data. The command `bw-takeout verify` checks a backup generation offline. Starting the official `bw` CLI, the interactive login and the per-run CLI profile are implemented, tested against a fake `bw` and exercised by hand against Vaultwarden on Linux and macOS. The command `bw-takeout backup` backs up one account interactively; it was run by hand against Vaultwarden on Linux and macOS. Bitwarden Cloud and Windows are untested, and there is no installable application yet.

- [User guide and documentation overview](docs/README.md)
- [Limits of the completeness check](docs/user/limitations.md)
- [Open the development environment and start Explore](docs/development/getting-started.md)

The project is licensed under the MIT license, see [LICENSE](LICENSE).

bw-takeout is an independent project. It is not affiliated with, endorsed by or sponsored by Bitwarden, Inc. Bitwarden is a trademark of Bitwarden, Inc.
