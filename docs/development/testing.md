# Test Strategy

For the first change, define a known synthetic reference inventory outside the CLI's view. Verify two passkeys with different keys and counters, then simulate a missing credential, a missing key, and an equal count with swapped content. Download two attachments with the same name but different IDs and bytes. Make one individual download fail and check that no silent success results.

Further cases: stale local state, a restricted organization role, interrupted encryption, and two concurrent runs with the same target. Existing archives must stay byte-identical. For offline recovery, decrypt without network access and compare the contents with the reference inventory. Report mock, SDK test and authenticated end-to-end test separately. These product tests are planned.

## Bootstrap Check of 13 November 2026

Docker build and start as user vscode on Linux/arm64 succeeded. Go 1.27.3, bw 2026.10.1, age 1.2.1 and OpenSpec 2.0.0 were run in the image. A synthetic age round trip succeeded without network access. SHA-256 of bw's installed npm entry point (build/bw.js): `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b`. Package integrity is recorded separately in the npm lockfile.

OpenSpec init generated six shared skills. The Claude reference for Explore is readable in the container. openspec list reported no active changes. For a temporary change, the proposal instructions contained the project-specific documentation rule. The VS Code UI, the personal Claude feature, Linux/amd64 and authenticated vaults were not tested.
