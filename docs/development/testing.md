# Teststrategie

Für den ersten Change einen bekannten synthetischen Sollbestand außerhalb der CLI-Sicht festlegen. Zwei Passkeys mit unterschiedlichen Schlüsseln und Countern prüfen, anschließend fehlendes Credential, fehlenden Schlüssel und gleiche Anzahl bei vertauschtem Inhalt simulieren. Zwei gleichnamige Anhänge mit verschiedenen IDs und Bytes abrufen. Einen Einzelabruf fehlschlagen lassen und prüfen, dass kein stiller Erfolg entsteht.

Weitere Fälle: veralteter lokaler Stand, eingeschränkte Organisationsrolle, unterbrochene Verschlüsselung und zwei gleichzeitige Läufe mit identischem Ziel. Bereits vorhandene Archive müssen bytegleich bleiben. Für Offline-Recovery ohne Netz entschlüsseln und Inhalte mit dem Sollbestand vergleichen. Mock, SDK-Test und authentifizierten End-to-End-Test getrennt ausweisen. Diese Produktprüfungen sind geplant.

## Bootstrap-Prüfung vom 13. November 2026

Docker-Build und Start als Benutzer vscode auf Linux/arm64 erfolgreich. Go 1.27.3, bw 2026.10.1, age 1.2.1 und OpenSpec 2.0.0 wurden im Image ausgeführt. Ein synthetischer age-Rundlauf gelang ohne Netzwerk. SHA-256 des installierten npm-Einstiegspunkts von bw (build/bw.js): `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b`. Die Paketintegrität ist separat im npm-Lockfile festgehalten.

OpenSpec init erzeugte sechs gemeinsame Skills. Der Claude-Verweis für Explore ist im Container lesbar. openspec list meldete keine aktiven Changes. Bei einem temporären Change enthielten die Proposal-Instruktionen die projektspezifische Dokumentationsregel. VS-Code-Oberfläche, persönliches Claude-Feature, Linux/amd64 und authentifizierte Tresore wurden nicht getestet.
