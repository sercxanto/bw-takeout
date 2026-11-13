# Integrations

The Dockerfile pins Debian-based Go and Node images by manifest digest. npm ci verifies the bw and OpenSpec packages against the integrity values in the lockfile. age and age-keygen come from the versioned Go module, verified by the Go checksum database. When updating, review pins and lockfile together, rebuild the image and repeat the relevant regression tests.

The unmodified bw version is deliberately pinned to the version used in the research. For later integration runs, use only isolated test profiles and synthetic accounts. Report Bitwarden Cloud and Vaultwarden separately. Run a Vaultwarden test instance outside the development container if needed.
