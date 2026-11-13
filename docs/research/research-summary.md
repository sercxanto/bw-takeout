# Research Overview

The following evidence levels refer to the included investigations of 31 October 2026. The bootstrap check does not repeat their experiments. The original reports are in German.

| Subject | Finding and scope |
|---|---|
| Passkey export | In the successful synthetic export path of the examined CLI version 2026.10.1, 13 native fields including keyValue and counter are confirmed. This does not imply that every backed-up passkey can be used to log in successfully. |
| Attachments | In the tested ZIP service path, name collisions and skipped download errors were reproduced. Plan individual downloads per item ID and attachment ID, and record results and errors per ID. |
| Silent omission | SDK/export service experiment with corrupt key material: an item or its credentials can be missing. Code analysis shows shared decrypted views for list and export. Matching counts and the exit code are not sufficient proof of completeness. |
| Second inventory | In the personal legacy test, account-bound encrypted_json showed two expected credentials despite an empty plaintext inventory. For the modeled blob, only the outer ID was visible. Equal counts do not prove equal content. |
| Freshness and permissions | Stale sync was modeled. Actual server completeness, organization permissions, access to organization attachments and Vaultwarden compatibility need separate live tests. |
| KDBX | The converter audit describes field losses and robustness problems. A later derivative needs its own loss and recovery tests. |

**Original sources:** [CLI analysis with release, code and test evidence](bitwarden-cli-analysis-2026-10-31.md), [completeness experiments with SDK, service and test doubles](completeness-decision-2026-10-31.md), [KDBX converter audit](kdbx-converter-audit-2026-10-31.txt). The originals stay unchanged apart from their ASCII file names. Add new results separately, with version, method and scope.
