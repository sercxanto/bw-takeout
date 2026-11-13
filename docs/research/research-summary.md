# Recherche einordnen

Die folgenden Belegstufen beziehen sich auf die mitgelieferten Untersuchungen vom 31.10.2026. Die Bootstrap-Prüfung wiederholt deren Experimente nicht.

| Gegenstand | Befund und Reichweite |
|---|---|
| Passkey-Export | Im erfolgreichen synthetischen Exportpfad der untersuchten CLI-Version 2026.10.1 sind 13 native Felder einschließlich keyValue und counter belegt. Daraus folgt keine erfolgreiche Anmeldung mit jedem gesicherten Passkey. |
| Anhänge | Im getesteten ZIP-Servicepfad sind Namenskollisionen und übergangene Downloadfehler reproduziert. Einzelabruf je Item-ID und Attachment-ID vorsehen, Ergebnisse und Fehler pro ID erfassen. |
| Stille Auslassung | SDK-/Exportservice-Experiment mit defektem Schlüsselmaterial: Ein Item oder seine Credentials können fehlen. Codeanalyse zeigt gemeinsame entschlüsselte Sichten bei Liste und Export. Übereinstimmung und Exitcode reichen nicht als Vollständigkeitsnachweis. |
| Zweites Inventar | Kontogebundenes encrypted_json zeigte im persönlichen Legacy-Test zwei erwartete Credentials trotz leerem Klartextbestand. Beim modellierten Blob war nur die äußere ID sichtbar. Gleiche Anzahl beweist keine inhaltliche Gleichheit. |
| Frische und Berechtigungen | Stale-Sync wurde modelliert. Tatsächliche Servervollständigkeit, Organisationsrechte, Zugriff auf Organisationsanhänge und Vaultwarden-Kompatibilität benötigen getrennte Live-Tests. |
| KDBX | Das Konverter-Audit beschreibt Feldverluste und Robustheitsprobleme. Eine spätere Ableitung benötigt eigene Verlust- und Recovery-Tests. |

**Originalquellen:** [CLI-Analyse mit Release-, Code- und Testnachweisen](Bitwarden-CLI-Analyse-2026-10-31.md), [Vollständigkeitsexperimente mit SDK, Service und Testdoubles](Entscheidung.md), [KDBX-Konverter-Audit](Eingefügter%20Text.txt). Die Originale bleiben unverändert. Neue Ergebnisse mit Version, Methode und Reichweite separat ergänzen.
