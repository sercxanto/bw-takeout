# Requirements

- Linux, macOS and Windows are target platforms. The first prototype handles one account interactively with TOTP. Multiple accounts, servers and organizations are part of the expansion goal.
- Personal vault data, passkeys including key material, and attachments belong in the archive. Server, account and organization scope must be clearly attributable.
- Each backup gets a new target. Even with concurrent runs, no older backup may be overwritten. Aborted runs must not leave a seemingly successful result.
- Export scope, errors and areas that cannot be verified must stay visible. An absolute completeness guarantee is not possible with the intended source.
- Offline readability must be tested with a separately stored key and a synthetic inventory.
