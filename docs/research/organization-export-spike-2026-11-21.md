# Spike: Organization Roles, Exports and Attachments on Vaultwarden 1.37.4

Date of the run: 21 November 2026 (UTC), during the explore session of the change `organization-export`. The spike asked how `bw` 2026.10.1 behaves toward an organization on the pinned Vaultwarden: which items an admin sees, whether the organization export works and for which roles, and what happens when a role without the export permission asks for it. No product code was written, and the product did not run. The code reading used the same `bw` bundle.

**Main result.** Owner and admin could export the organization in both formats, and both exports held the same item IDs with the same `revisionDate`, which also matched `bw list items`. An admin assigned to one of two collections saw the items of both collections locally and downloaded both attachments. A user and a manager got 401 from Vaultwarden on the organization export, and `bw` then logged itself out, so that every later command failed as not logged in.

## Versions and artifacts

| Component | Value |
|---|---|
| Bitwarden CLI | 2026.10.1, npm package from the dev container. SHA-256 of `build/bw.js`: `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b` (same as the [live probe](vaultwarden-live-probe-2026-11-14.md)) |
| Host | Dev container, Linux aarch64 |
| Server | Vaultwarden 1.37.4, image `docker.io/vaultwarden/server:1.37.4-alpine`, linux/arm64 |
| linux/arm64 server binary SHA-256 | `ae658947f85964e934d7673d9cab4c0edbce8e6167422d6ff3b10008e125421d`, hashed from the cache of the Vaultwarden suite (`.local/vaultwarden-cache/`), the same binary as in the [registration spike](vaultwarden-registration-spike-2026-11-21.md) |
| Database | SQLite, created by the server on first start |

## Method

**Server and accounts.** The server ran as in the [registration spike](vaultwarden-registration-spike-2026-11-21.md): on loopback with TLS, sign-ups allowed, no SMTP. The spike first registered three synthetic accounts over HTTP (owner, admin and user) and later a fourth (manager), each with its own `bw` profile.

**Organization.** The owner created the organization over the API, with the calls recorded in design D12 of the change `organization-export`:

1. `POST /api/organizations` with a new 64-byte organization key wrapped for the owner's public key (an EncString of type 4, RSA-OAEP with SHA-1), a new RSA key pair of the organization, and the default collection C1.
2. `POST /api/organizations/{id}/collections` for a second collection C2.
3. `POST /api/organizations/{id}/users/invite` for each member, with `accessAll: false` and C1 as the only collection. Without SMTP, an existing account became accepted (status 1).
4. `POST /api/organizations/{id}/users/{memberId}/confirm` with the organization key wrapped for the member's public key.

The owner then created through `bw` one login item in C1 and one in C2, each with an attachment of random bytes, and a third item in C1 that was deleted to the trash.

**Commands.** Each member ran `bw sync`, `bw list organizations`, `bw list items`, `bw get attachment` for each attachment shown, and `bw export --organizationid <id>` with `--format json` and `--format encrypted_json`.

**Secrets handling.** The addresses were synthetic and the passwords random. Passwords, session keys, account keys and the organization key stayed in memory or in temporary files that were removed. Outputs were inspected for status codes, IDs, counts, roles, revision dates and structure only. No password, token, session key, organization key or ciphertext appears in this note.

## Results

Observed in the run:

| Statement | Result |
|---|---|
| Admin's local view | An admin with `accessAll: false`, assigned to collection C1 only, saw the items of C1 and C2 in `bw list items` and downloaded both attachments with matching SHA-256 |
| Export as owner and admin | Both organization exports succeeded and held the same IDs with the same `revisionDate`, which also matched `bw list items` |
| Trash | Both organization exports left out the item in the trash |
| Form of the exports | The plaintext organization export had the keys `encrypted: false`, `collections` and `items`. The encrypted one had `encrypted: true`, `encKeyValidation_DO_NOT_EDIT`, `collections` and `items`, with sealed values and an item key per item |
| Export as user and manager | `bw export --organizationid` got 401 from Vaultwarden, `bw` retried and then logged out. Every later command printed "You are not logged in.", and `bw status` reported `unauthenticated`. Run twice |
| Roles in `bw list organizations` | Owner 0, admin 1 and user 2. A manager invited with type 3 was reported as type 4. A confirmed membership had status 2 |

## Code reading

From the `bw` 2026.10.1 bundle (`build/bw.js`, hash above):

| Statement | Bundle lines |
|---|---|
| `bw export --organizationid` in both formats calls `GET /organizations/{id}/export` on every call and does not read the local snapshot | 58904, 58946, 59794 |
| The CLI calls `getOrganizationExport` without `onlyManagedCollections`, so the whole organization is exported | 58643, 59868 |
| Both organization exports leave out items with a `deletedDate` | 58918, 58958 |
| Export items carry `revisionDate` and `collectionIds`, but no attachments | `CipherExport.build`, 56104 ff. |
| The encrypted organization export is built by `BuildEncryptedExport` with the organization key | 59016 ff. |
| The personal exports drop every item with an `organizationId` while building the document | 59358 and `getEncryptedExport` |
| `ExportCommand` checks no permission. Only the server decides | 59830 ff. |

## Scope

- The findings hold for `bw` 2026.10.1 from npm on Linux arm64 against Vaultwarden 1.37.4 with SQLite, for one organization with two collections and legacy items, in one run (the 401 path in two).
- The admin's view of collections the admin is not assigned to is Vaultwarden's behavior. On Bitwarden Cloud it depends on an organization setting.

## Not tested

- Bitwarden Cloud.
- Custom roles with an export permission, and how `bw list organizations` reports them.
- Organization items in blob format, and passkeys in organization items.
- An organization export with decryption error lines.
- An organization that changes during a run.
- The product itself: no `bw-takeout backup` or `verify` ran.

## Status of the scripts

The spike scripts were lost in a scratchpad cleanup. The results above are those recorded in the explore session. The organization setup that worked is recorded in design D12 of the change `organization-export`, and the Vaultwarden scenario of that change rebuilds it in `internal/e2e`.

## Cleanup

The server, its data directory, the certificate, the `bw` profiles and the seed files were removed after the run. Only this note and one row in the research overview remain in the repository.
