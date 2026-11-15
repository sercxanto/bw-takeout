package archive

// RecoveryText is the content of RECOVERY.txt. It is fixed, so it holds no
// data from the vault.
const RecoveryText = `BW-TAKEOUT GENERATION
===========================

This archive holds one backup of a personal Bitwarden vault, made with the
official Bitwarden CLI (bw) and stored by bw-takeout. It can be opened
without bw-takeout.


LAYOUT

RECOVERY.txt
    This file. It is the first entry and contains no data from the vault.

documents/export.json
    The plaintext JSON export of the vault, byte for byte as bw wrote it.
    It contains every password, note, field and passkey of the personal
    items in clear text. This is the document to restore from or to
    convert: Bitwarden, Vaultwarden and several other password managers
    can import it.

documents/export.encrypted.json
    The account-bound encrypted JSON export. It can only be read with the
    key of the account that made it, and only imported into that account.
    It is kept as evidence of what the server returned.

documents/items.json
    The item list from "bw list items". It is the only document that names
    the attachments of each item.

attachments/<item-id>/<attachment-id>
    The content of one attachment, unchanged. The directory is the ID of
    the item the attachment belongs to, the file is the ID of the
    attachment. Look up the item ID in documents/export.json to find the
    item. The original file name is in manifest.json.

manifest.json
    The last entry. It states when and from which server and account the
    backup was made, which program versions made it, the size and SHA-256
    of every other entry, and for attachments the item ID, attachment ID
    and original file name. Under "verification" it holds the completeness
    verdict computed when the backup was made: "verified", or
    "unverified" with the findings that kept it from being verified.


OPENING THE ARCHIVE

The archive file vault.tar.age is a tar file encrypted with age
(https://age-encryption.org). You need the age identity (secret key) that
belongs to one of the recipient keys the backup was encrypted to.

To check the archive without writing any plaintext:

    age -d -i identity.txt vault.tar.age | tar -tvf -

To unpack it into the current directory:

    age -d -i identity.txt vault.tar.age | tar -xf -

Unpacking writes the whole vault in clear text to disk. Do it only on an
encrypted or temporary volume, and delete the files when you are done.


CHECKING INTEGRITY

Compare the SHA-256 of each unpacked file with the value in manifest.json,
for example with "sha256sum documents/export.json". The command
"bw-takeout verify --identity identity.txt <generation-directory>"
performs all checks without writing any file.
`
