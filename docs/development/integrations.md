# Integrationen

Das Dockerfile pinnt Debian-basierte Go- und Node-Images per Manifest-Digest. npm ci prüft die Pakete von bw und OpenSpec gegen die Integritätswerte im Lockfile. age und age-keygen stammen aus dem versionierten Go-Modul mit Go-Prüfsummendatenbank. Bei Aktualisierungen Pins und Lockfile gemeinsam prüfen, Image neu bauen und die relevanten Regressionstests wiederholen.

Die unveränderte bw-Version ist absichtlich auf den Research-Stand festgelegt. Für spätere Integrationsläufe ausschließlich isolierte Testprofile und synthetische Konten verwenden. Bitwarden Cloud und Vaultwarden separat ausweisen. Eine Vaultwarden-Testinstanz bei Bedarf außerhalb des Entwicklungscontainers betreiben.
