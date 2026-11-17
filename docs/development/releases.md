# Releases

The license is settled: MIT, with Georg Lutz as copyright holder, see [LICENSE](../../LICENSE).

Third-party notices: the dependencies age (`filippo.io/age`), `filippo.io/hpke`, `golang.org/x/crypto`, `golang.org/x/sys` and `golang.org/x/term` are under three-clause BSD licenses. `filippo.io/hpke` came with age 1.3. `golang.org/x/term` is a direct dependency for terminal detection and for restoring the terminal state around the login. A binary release must ship their license notices. Collecting them is a release task.

No product artifacts exist yet. Before a first release, document platform tests, the offline read test, known limitations and the verified bw version. Distribution and signing are open.
