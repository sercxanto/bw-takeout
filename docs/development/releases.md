# Releases

The license is settled: MIT, with Georg Lutz as copyright holder, see [LICENSE](../../LICENSE).

Third-party notices: the dependencies age (`filippo.io/age`), `filippo.io/hpke`, `golang.org/x/crypto`, `golang.org/x/sys` and `golang.org/x/term` are under three-clause BSD licenses. `filippo.io/hpke` came with age 1.3. `golang.org/x/term` is a direct dependency for terminal detection and for restoring the terminal state around the login. A binary release must ship their license notices. Collecting them is a release task.

No product artifacts exist yet. Before a first release, document platform tests, the offline read test, known limitations and the verified bw version. Distribution and signing are open.

Before a release, start the workflow `Vaultwarden` ([`vaultwarden.yml`](../../.github/workflows/vaultwarden.yml)) by hand on the release commit and require it to pass. It runs the [Vaultwarden tests](testing.md#vaultwarden-tests) against a live server on Linux amd64. Record the run in the test strategy with the commit, the duration and both scenarios. The workflow never runs on its own, so a passing `CI` and `Checks` do not cover it.
