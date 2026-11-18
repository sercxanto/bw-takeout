# Code Notes: Sync Behavior of list, export and get in bw 2026.10.1

Date: code reading on 17 November 2026 (UTC). These notes collect the findings that the change `capture-collection` builds on: whether `bw list items`, `bw export` or `bw get attachment` start a sync, and whether the CLI runs a sync outside a command. No account, no server and no secret was involved, and `bw` was not started.

## Versions and artifacts

| Component | Value |
|---|---|
| Bitwarden CLI | 2026.10.1, npm package `@bitwarden/cli` in the dev container |
| Bundle | `build/bw.js`, 97082 lines, SHA-256 `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b` |

The SHA-256 matches the one in the package documentation of `internal/capture` and in the [code notes of 15 November 2026](bw-cli-code-notes-2026-11-15.md).

## Method

The installed bundle was read with a text editor and `grep`. Read in full:

- `ListCommand` (58218 to 58458),
- `ExportCommand` (59834 to 59949),
- `VaultExportService` (58604 to 58702),
- the personal export paths of `IndividualVaultExportService` (59107 to 59372),
- `GetCommand.run`, `getCipherView`, `getCipher` and `getAttachment` (57662 to 57946),
- `DownloadCommand.saveAttachmentToFile` (50034 to 50080),
- the cipher service methods these call, and the event collection and upload methods,
- `BaseProgram.exitIfLocked` and `handleLockedUser` (32194 to 32235),
- the start of the process: `ServiceContainer.init` (96973 to 97004) and `main` (97066 to 97075).

To find every sync, the bundle was searched for every occurrence of `fullSync` and for the notification handlers of the sync service. Each quoted expression below was looked up with `grep -n` on 17 November 2026 and found at the line given.

Code reading shows what the code would do. It does not show what happens at run time, which server flags apply, or what other versions do.

## Findings

Evidence level of every finding: **code reading**.

| # | Finding | Quoted expression | Line |
|---|---|---|---|
| 1 | Every sync goes through `syncService.fullSync`. The bundle has 17 occurrences of `fullSync(`: the definition at 72925, one call without `yield` in the import command at 60075, and the 15 calls listed in finding 2 | `yield this.syncService.fullSync(` | 38596, 39134, 45555, 60117, 64161, 64169, 64183, 64225, 64231, 64309, 69190, 69286, 69367, 69417, 69572 |
| 2 | The calls lie in five places, none of them inside `list`, `export` or `get`. The places are the login success handler and `LoginCommand` (38596, 39134), `SyncCommand` (45555), `ImportCommand` (60075, 60117), and `OrganizationApiService` in `create`, `createWithoutPayment`, `save`, `leave`, `delete` and `updateCollectionManagement` (64161 to 64309). The last place is the account migrations (69190 to 69572) | see finding 1 | see finding 1 |
| 3 | `bw list items` reads the decrypted items from local state and starts no request for items | `ciphers = yield this.cipherService.getAllDecrypted(activeUserId);` | 58265 |
| 4 | Both personal exports read local state: the plaintext export reads the decrypted items, the encrypted export reads the stored encrypted items | `promises.push(this.cipherService.getAllDecrypted(activeUserId).then((ciphers) => {` and `promises.push(this.cipherService.getAll(activeUserId).then((c) => {` | 59247, 59278 |
| 5 | The stored items are read from the state of the account, not from the server | `const ciphers = yield (0,external_rxjs_namespaceObject.firstValueFrom)(this.ciphers$(userId));` | 77733, 77746 |
| 6 | `bw get attachment` takes the item and its attachment metadata from local state. It then asks the server for a download URL and fetches the content from it. Neither request writes items to the state | `const cipher = yield this.getCipherView(itemId, activeUserId);`, `yield this.apiService.getAttachmentData(cipher.id, attachments[0].id);` and `const response = yield this.apiService.nativeFetch(new Request(url, { headers: { cache: "no-cache" } }));` | 57910, 57938, 50053 |
| 7 | All three commands record an event and upload the recorded events right away. The upload posts events and starts no sync | `yield this.eventCollectionService.collectMany(EventType.Cipher_ClientViewed, ciphers, true);`, `this.eventCollectionService.collect(eventType, null, true, options.organizationid);`, `yield this.eventUploadService.uploadEvents();` and `failedEvents = yield this.apiService.postEventsCollect(eventRequests, userId);` | 58320, 59874, 57780, 68296, 68324, 68446 |
| 8 | The account migrations sync in two ways. `needsMigration` of three migrations calls `fullSync(false)` before it decides, and the migrator calls `fullSync(true)` after any migration ran | `if (ranMigration) {` followed by `yield this.syncService.fullSync(true);` | 69190, 69286, 69367, 69571 to 69572 |
| 9 | The migrations run only from the login success handler, `LoginCommand` and `UnlockCommand`. This is finding 16 of the code notes of 15 November 2026, together with the unlock | `yield this.encryptedMigrator.runMigrations(` | 31998, 38600, 39147 |
| 10 | `list`, `export` and `get` start with `exitIfLocked`. A locked vault runs `UnlockCommand`, and with it the migrations, only when interaction is allowed. With `BW_NOINTERACTION=true` the command ends with "Vault is locked." instead | `if (process.env.BW_NOINTERACTION === "true") {` | 32219; `yield this.exitIfLocked();` at 61221, 61276, 61516 |
| 11 | The start of every process unlocks with the session key from the environment, which runs no migration. It also runs the storage migrations, which contain no `fullSync` call | `yield this.unlockService.unlockWithAutoUnlockKey(activeAccount.id);` and `yield this.migrationRunner.run();` | 96997, 96981 |
| 12 | The CLI starts no timer that syncs. The two interval timers check the vault timeout and upload events, and each starts only from an `init` method that the CLI never calls | `this.taskSchedulerService.setInterval(ScheduledTaskNames.vaultTimeoutCheckInterval, 10 * 1000);` and `this.taskSchedulerService.setInterval(ScheduledTaskNames.eventUploadsInterval, 60 * 1000);` | 35533, 68417 |
| 13 | The notification handlers of the sync service, which would apply server pushes to the state, are defined but never called. The bundle holds no notification connection | `syncUpsertCipher(notification, isEdit, userId) {` | 72727, the only occurrence of `syncUpsertCipher(` |

Consequences for a capture:

- After one `bw sync --force`, the plaintext export, the encrypted export and both item lists read the same local snapshot (findings 1 to 5). No command between them can change it, so a comparison of the inventory before and after the capture is not needed.
- A download reads the attachment metadata from that snapshot. The content comes from the server at the time of the download (finding 6). An attachment that was replaced on the server between the sync and the download would not match the snapshot. Its content is then still bound to the ID pair the snapshot names.
- The migrations, and their syncs, cannot run inside `list`, `export` or `get` as long as `BW_NOINTERACTION=true` is set (findings 8 to 10). `internal/bwcli` sets it on every command except the login.
- The design of `capture-collection` names create and edit among the callers of `fullSync`. The bundle shows no call in `CreateCommand` or `EditCommand`; the calls besides login, unlock, `sync` and import are in the organization API service (finding 2). This does not change the conclusion.

## Scope

- The findings hold for this one bundle. A later `bw` version can change any of them.
- Only the personal paths were read. The organization export and `bw serve` were not examined for this note.
- The search for syncs relies on the name `fullSync`. A sync that writes items to the state without that method would not be found. The calls of `cipherService.replace`, `upsert`, `clear` and `delete` found by `grep` lie in the sync service, the archive service, the folder service and the logout. Writes from inside the cipher service, such as those of create and edit, were not traced.

## Not observed at run time

- That no sync request reaches the server during `list`, `export` or `get`. No network trace was taken.
- Which requests these commands send besides the attachment download and the event upload, for example a fetch of the server configuration.
- An attachment that changed on the server between the sync and its download.
- A run in which an account migration was pending.
