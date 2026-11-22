# Configuration

There is no configuration file yet. Until there is one, the flags of `bw-takeout backup` are the configuration: the server, the target, the recipients file, and optionally the `bw` executable, a certificate authority file and the organizations to back up. See [usage](usage.md#making-a-backup). One run backs up one account.

The organizations are chosen by ID with the repeatable flag `--organization`, once per organization, in the order the run handles them. They are never chosen automatically: an organization the account belongs to is backed up only when its ID is given, and no warning names the others. See [usage](usage.md#organizations).

The format of a configuration file is open. It is planned together with several accounts and batch operation. Server and account mapping, the organizations per account, archive target, key management and proxy settings need to be documented then. There are no binding configuration keys yet.
