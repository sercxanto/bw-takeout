# Integrations

The Dockerfile pins Debian-based Go and Node images by manifest digest. npm ci verifies the bw and OpenSpec packages against the integrity values in the lockfile. age and age-keygen come from the versioned Go module, verified by the Go checksum database. When updating, review pins and lockfile together, rebuild the image and repeat the relevant regression tests.

The unmodified bw version is deliberately pinned to the version used in the research. For later integration runs, use only isolated test profiles and synthetic accounts. Report Bitwarden Cloud and Vaultwarden separately. Run a Vaultwarden test instance outside the development container if needed.

The age library `filippo.io/age` in `go.mod` and the age CLI installed by the Dockerfile are pinned to the same version, v1.2.1. Update both together, because the recovery test checks archives written by the library against the CLI. `go.sum` holds the checksums of the library and its dependencies.

Adding or updating a module needs a module cache that the container user can write to. The Dockerfile makes the content of `/go` belong to that user after the tools are installed. A container built before that change has a cache owned by root, so `go mod download` fails there until the image is rebuilt.
