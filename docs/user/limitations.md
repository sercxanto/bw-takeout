# Limitations

A successful CLI export can deliver decryptable partial inventories while items or passkeys are missing. Missing organization permissions and stale local state limit the view further. An independent reference inventory is often unknown. See [evidence levels](../research/research-summary.md).

## What a Verdict Can and Cannot Detect

Every capture is judged failed, unverified or verified ([ADR 0004](../decisions/0004-completeness-verdict.md)). The verification is planned and not yet part of a usable tool. Its rules are tested with synthetic data only. No live capture backs them yet.

It detects:

- an item that is in the encrypted export but missing from the plaintext export, and the reverse,
- a passkey missing from an item whose passkey count the encrypted export shows,
- a passkey with a missing field, an empty or unparsable key, or a counter that is not a non-negative number,
- an attachment that failed to download, has no download result, or has a result that the item list does not explain,
- a personal item that the item list shows and the plaintext export lacks.

It cannot detect:

- **Missing passkeys in blob-format items.** The encrypted export seals the content of such items, so their passkey count is not visible. A blob login item is reported as unverified, never as "no passkeys".
- **A swapped passkey or a wrong key without a reference.** If a passkey is replaced by another valid passkey, or a key is well-formed but wrong, the counts and the structure still match. Every result without a reference inventory lists credential identity and attachment content as not checked.
- **A missing attachment entry.** The decrypted item list is the only source of attachment metadata. An attachment that is missing from an otherwise intact list entry stays undetected. Every result without a reference inventory lists this limit.
- **Items the item list does not show.** Such items are reported as unverified, because their attachments are unknown. A default listing leaves out archived items, so the listing of archived items has to be included.
- **Completeness against the server.** "Verified" covers the personal items in the local snapshot that the CLI exported during the capture. An omission in a step that both exports share, and items that the CLI leaves out of every document, stay invisible. Every result states this scope and lists it as a limit.

Organization items are not checked. They are counted per organization and reported as a warning.
