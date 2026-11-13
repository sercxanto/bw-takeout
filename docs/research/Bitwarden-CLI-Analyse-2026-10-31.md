# Bitwarden CLI als Grundlage eines portablen Backup-Tools

Stand: **31.10.2026**. Untersuchung der offiziellen CLI, keine Implementierung des Backup-Tools. Keine echten Zugangsdaten, Konten oder Tresorinhalte verwendet.

**Entscheidung: bedingtes GO für `bw` als Login-, Entschlüsselungs- und Exportkomponente. NO-GO für ein produktives Vollständigkeitsversprechen allein auf Basis von `bw export`, dessen Exit-Code oder einem Vergleich mit `bw list`. Den eingebauten ZIP-Export empfehle ich nicht als primäres Backupformat.**

Die erforderlichen Passkey-Daten sind grundsätzlich vorhanden. Die entscheidenden Hindernisse sind stille Auslassungen, ZIP-Namenskollisionen und die fehlende unabhängige Vollständigkeitskontrolle. JSON plus einzeln über IDs heruntergeladene Anhänge ist die bessere Grundlage, löst aber die stille Auslassung bei der Entschlüsselung noch nicht allein.

## 1. Verifizierter Versionsstand und Untersuchungsumfang

| Komponente | Verifizierter Stand |
|---|---|
| Offizieller CLI-Release | **2026.10.1**, veröffentlicht 29.10.2026, 12:21 UTC; jüngster CLI-Eintrag in der abgefragten Release-Liste |
| `bitwarden/clients` | `91214c3b978edb5496a32a946451960fe7d44531`, ausgecheckter Release-Commit |
| CLI-Paketdatei | `apps/cli/package.json` nennt ebenfalls `2026.10.1` |
| Eingebundenes SDK | `@bitwarden/sdk-internal` und kommerzielles Pendant: `0.2.0-main.1126` |
| Öffentliches SDK | npm-Metadaten nennen `gitHead` **`4a9eb239fa7f63132ad7a32cce9aeb5fae252243`**; relevante Rust-Dateien an diesem SHA geprüft |
| Vaultwarden | Jüngster abgefragter Release **1.37.4**, Tag auf Commit `7c188967e7a467b19b2757bc3326f9cf3ccae327` |
| Lokal ausgeführte CLI | Offizielles `bw-macos-arm64-2026.10.1.zip`; `bw --version` bestätigt 2026.10.1 |

SHA-256 des heruntergeladenen CLI-Archivs: `4e595f9da80ef8b30abca808d0c78a1560d9589466d1bccd2b3e70f321fb12bf`. Stimmt mit dem Digest des GitHub-Release-Assets überein. Das ist ein Integritätsvergleich mit den Release-Metadaten, kein unabhängiger Reproducible-Build-Nachweis.

Quellen: [CLI-Release][release], [CLI-Paket][package], [SDK-Pinning][package-root], [SDK-Metadaten][sdkmeta], [Vaultwarden-Release][vwrelease]. Der referenzierte frühere Chat und sein Textanhang wurden gelesen; frühere Aussagen wurden nicht ungeprüft übernommen.

**Nachweisstufen:** „Code“ bedeutet Prüfung des angegebenen, gepinnten Quellcodes; „Mock“ bedeutet Ausführung originaler transpilierten TypeScript-Funktionen mit ersetzten Dienst-/Netzwerkgrenzen; „Binary“ bedeutet Ausführung der offiziellen macOS-Binärdatei. Nicht durchgeführt wurden Anmeldung an einem echten Testserver, vollständige CLI-Sicherung eines Testkontos, WebAuthn-Anmeldung nach Import, Windows-/Linux-Lauf oder die gesamte Upstream-Testsuite. Kein vollständiger CLI-/Rust-Neubau.

## 2. JSON, Passkeys, eigene Felder und ZIP

**Passkeys: ja, einschließlich Schlüsselmaterial und Zähler — für erfolgreich entschlüsselte, zum Export zugelassene Einträge.**

`LoginExport` überträgt das gesamte vorhandene `fido2Credentials`-Array. `Fido2CredentialExport` enthält diese 13 bekannten Felder:

`credentialId`, `keyType`, `keyAlgorithm`, `keyCurve`, `keyValue`, `rpId`, `userHandle`, `userName`, `counter`, `rpName`, `userDisplayName`, `discoverable`, `creationDate`.

`keyValue` ist privates Schlüsselmaterial; das bezeichnet auch der SDK-Kommentar ausdrücklich so. Der Zähler wird im JSON als Zeichenkette gespeichert, beispielsweise `"6"`; `discoverable` ebenfalls als Zeichenkette. Es wird nichts absichtlich auf einen neuen Zähler zurückgesetzt. Unbekannte zukünftige Felder werden durch dieses explizite Mapping allerdings nicht automatisch übernommen. [Passkey-Export][fido], [Login-Export][loginexport], [SDK-Modell][sdklogin]

Im Mock-Test wurde ein synthetischer P-256-Schlüssel erzeugt, als PKCS#8 in `keyValue` eingesetzt und durch den originalen JSON-Export geführt. Alle 13 Felder, Zähler `6` und ein verstecktes eigenes Feld blieben erhalten. Der aus dem Export geladene private Schlüssel signierte anschließend eine Nachricht, deren Signatur mit dem ursprünglichen öffentlichen Schlüssel verifiziert wurde. **Das belegt die Serialisierung und kryptografische Brauchbarkeit dieser Fixture, nicht einen vollständigen WebAuthn- oder Server-Roundtrip.**

Eigene Felder werden mit `name`, `value`, `type` und gegebenenfalls `linkedId` exportiert. Native Strukturen für Login, Notizen, Karten, Identitäten, SSH-Schlüssel sowie weitere neue Itemtypen sind im Mapper vorhanden. Passwort-Historie und Zeitstempel sind ebenfalls modelliert. Dies ist trotzdem kein vollständiges Server-/Account-Backup: Einstellungen, Mitgliedschaften, Organisationsrichtlinien und sämtliche internen Zustände gehören nicht automatisch dazu. [Cipher-Mapper][cipherexport], [Feld-Mapper][fieldexport]

| Format | Passkeys / eigene Felder | Anhänge | Bewertung |
|---|---|---|---|
| Persönliches JSON | Ja, im erfolgreichen Exportpfad | Keine Binärdaten; der Exportmapper enthält auch kein Attachment-Inventar | Geeigneter nativer Datenkern |
| Organisations-JSON | Gleicher Cipher-/Passkey-Mapper; Collections statt persönlicher Ordner | Keine Binärdaten | Separat pro Organisation sichern |
| Persönliches ZIP | `data.json` wird über dieselbe JSON-Funktion erzeugt | Entschlüsselte Dateien unter `attachments/` | Wegen nachgewiesener Fehler nicht als primärer Weg |
| Organisations-ZIP | Nicht unterstützt | Nicht unterstützt | Expliziter Fehler |
| CSV | Keine geeignete Passkey-Sicherung | Nein | Für diesen Zweck ausgeschlossen |
| Passwortgeschütztes JSON | JSON-Inhalt mit zusätzlicher Bitwarden-Verschlüsselung | Nein | Portabler als accountgebundene Variante, aber kein Gesamtarchiv |

Im Mock-Test war ZIP-`data.json` bei unverändertem Bestand exakt gleich dem separaten JSON-Export. ZIP ergänzt keine verlustfreie ID-basierte Attachment-Zuordnung: Verzeichnisse entstehen aus bereinigten Eintragsnamen. Persönliche Exporte schließen Organisationsdaten aus. Papierkorb und Sends bleiben entsprechend dem gewünschten Scope ausgeschlossen. [Persönlicher Export][individual], [Organisations-Export][orgexport]

**Vier konkrete ZIP-Fallen, mit originalen Methoden reproduziert:**

1. Ein fehlgeschlagener Attachment-Download wird abgefangen; das ZIP wird trotzdem gebaut. Der Service setzt `skippedAttachmentCount`, aber `ExportCommand.saveFile()` ignoriert diesen Wert und liefert beim Speichern Erfolg. Ein Fehler kann im Log stehen, ist aber kein fehlgeschlagener Backup-Lauf. [ZIP-Schleife][individual], [CLI-Speicherung][exportcmd]
2. Die Eintragsnamen `a/b` und `a_b` ergeben beide `attachments/a_b/`. Bei jeweils `same.txt` bleibt nur die zweite Datei. Die Duplikaterkennung zählt ursprüngliche Namen, nicht den tatsächlich verwendeten Pfad. Kein Skip-Zähler, kein Fehler.
3. Anhänge `x.txt`, `x.txt`, `x_1.txt` desselben Items ergeben nur zwei Dateien: Der erzeugte Suffixname kollidiert mit dem späteren Originalnamen. Wieder kein Skip-Zähler.
4. Ein durch „Restricted Item Types“ ausgeschlossener Eintrag fehlt in `data.json`, sein Anhang wird trotzdem aufgenommen. Die beiden Schleifen verwenden unterschiedliche Filter. Dies wurde auch als [Upstream-Issue #24335][issuezip] gemeldet; meine Reproduktion erfolgte mit Mocks, nicht gegen Bitwarden Cloud.

Zusätzlich: `--format zip --password <Wert>` ist kein verschlüsseltes ZIP. Die übergeordnete Servicefunktion verzweigt bei einem nichtleeren Passwort zum passwortgeschützten JSON. Auch diese Verzweigung wurde reproduziert. Der Wrapper muss diese Kombination verhindern. [Format-Routing][exportservice]

**Wiederverwendbarkeit von Passkeys:** Bitwarden dokumentiert JSON-Passkey-Import seit dem Release-Zyklus 2026.2.0. Der aktuelle Importer reicht die Daten durch `CipherWithIdExport.toView()` weiter. Das ist kein Beleg für identisches Verhalten jedes Vaultwarden-/Client-Paars. Bei älteren Passkeys mit steigendem Zähler kann ein korrekt gesicherter, später veralteter Zähler vom Zielservice abgewiesen werden. Vollständiger Export bedeutet deshalb nicht garantierte langfristige Verwendbarkeit. Zähler nicht eigenmächtig ändern. [Release Notes][notes], [Export-Dokumentation][exportdoc], [Importer][importer]

## 3. Aufrufe und Organisationsgrenzen

Diese Aufrufe sind der vorgesehene Ablauf, keine hier durchgeführte Kontoanmeldung. Das Profil wird vorher pro Account und Server getrennt gesetzt; eine entsperrte Session wird nur an die jeweiligen Kindprozesse übergeben.

```text
bw login --raw
bw sync --force
bw status
bw export --format json --output <neue-staging-datei>
bw list organizations
bw export --organizationid <org-id> --format json --output <neue-org-datei>
bw get item <item-id>
bw get attachment <attachment-id> --itemid <item-id> --output <id-basierter-pfad>
bw lock
```

Für das Attachment-Inventar liefert `bw get item` beziehungsweise `bw list items` zusätzliche Metadaten, die der native Export nicht enthält. IDs statt Dateinamen verwenden und Ausgabepfade selbst bestimmen. Archivierte Items ausdrücklich berücksichtigen: Die Standardliste filtert nach Archivstatus; ein einfacher Abgleich der Standardliste mit dem Export kann bereits deshalb abweichen. Abruf über die exportierten Item-IDs vermeidet die reine Namenssuche. [Get-Befehl][get], [List-Befehl][list], [CLI-Antwortmodell][cipherresponse]

**Organisationsrechte sind eine echte Grenze:** Der Organisations-Export ruft `/organizations/{id}/export` auf. Der CLI-Aufruf setzt `onlyManagedCollections` nicht und nutzt damit den organisationsweiten Pfad. Die im gemeinsamen Service vorhandene Variante für verwaltete Collections ist dadurch nicht automatisch als CLI-Option verfügbar. Exportrechte und Zugriff auf einzelne Anhänge müssen separat verifiziert werden. [API-Aufruf][orgapi], [Routing][exportservice], [CLI-Optionen][vaultprogram]

`bw get attachment` erwartet ein im lokalen Accountbestand auffindbares Item. Deshalb ist „Admin kann die Organisation exportieren“ **keine hinreichende Zusage**, dass er jeden Anhang über diesen CLI-Befehl herunterladen kann. Der MVP muss alle exportierten IDs abgleichen und bei nicht erreichbaren Items abbrechen; keine vollständige Organisationssicherung behaupten, wenn nur eine Teilmenge sichtbar ist. Der Code erlaubt Organisationsanhänge grundsätzlich ohne persönliche Premium-Berechtigung, verlangt für persönliche Anhänge aber Premium-Zugriff im Einzel-Downloadpfad. Rechte-/Lizenzfälle gehören in den Live-Test. [Attachment-Abruf][get]

## 4. Fehlerbehandlung, Konsistenz und Session-Sicherheit

| Bereich | Belegter Befund | Konsequenz |
|---|---|---|
| Exit-Codes | Normale `Response`-Fehler → 1, Erfolg → 0 | Zusätzlich Inhalte und erwartete Artefakte prüfen; keine stabilen differenzierten Fehlercodes erwarten |
| `BW_CLEANEXIT` | Jeder nichtleere Wert bewirkt 0, sogar `"false"` | Variable vollständig aus der Kindprozessumgebung entfernen; Binary-Test bestätigt |
| Login | Enthält automatischen Sync, ohne `allowThrowOnError: true`; dessen Boolean wird nicht geprüft | Login-Erfolg ist kein Frischebeleg; expliziten erzwungenen Sync danach verlangen |
| `sync` | Fordert Fehlerweitergabe an, ignoriert aber den Boolean-Rückgabewert | `false` führt im Mock trotzdem zu „Syncing complete“; `false` kann auch legitimes „kein Sync nötig“ bedeuten |
| Persönlicher Export | Liest entschlüsselten lokalen Bestand; führt nicht selbst einen Vollsync durch | Alten Cache nicht versehentlich als frisches Backup freigeben |
| Organisations-Export | Holt Daten separat über die Export-API | Persönlicher und Organisationsbestand bilden keine gemeinsame Servertransaktion |
| Einzel-Download | HTTP ungleich 200 wird Fehler; Entschlüsselungs-/Speicherfehler werden als Fehler zurückgegeben | Besser kontrollierbar als ZIP, aber Datei, erwartete ID und Ergebnis zusätzlich prüfen |
| Speichern | `fs.writeFile`, kein exklusives Erzeugen und keine atomare Veröffentlichung | Alte Backups niemals direkt als Ziel verwenden; erst neue Generation prüfen |

Quellen: [Exit-Code-Verarbeitung][baseprogram], [Login-Sync][logincommand], [Sync-Befehl][synccommand], [Sync-Service][syncservice], [Download][download], [Dateischreiben][utils]. Ein erfolgreicher `status`-Aufruf liefert auch `unauthenticated` mit Exit 0; Statusfeld tatsächlich lesen. Startup-Meldungen über eine unerreichbare Serverkonfiguration erschienen selbst bei `--version` auf stderr. Daher ist „stderr nicht leer“ ebenfalls keine zuverlässige Fehlerklassifikation.

**Die größte verbleibende Lücke liegt vor dem Export.** `CipherService.getAllDecrypted()` gibt nur erfolgreiche Entschlüsselungen weiter. Beim SDK-Pfad wird die Fehlerliste separat gespeichert; eine Exception kann sogar in `[]` umgewandelt werden. Der Export prüft diesen Fehlerbestand nicht. [Cipher-Service][cipherservice], [SDK-Brücke][sdkbridge]

Im gepinnten Rust-SDK existieren zwei Pfade: Der tolerante `Login`-Decryptor wandelt unter anderem fehlerhafte `fido2_credentials` mittels `.ok()` in einen fehlenden Wert um; `StrictDecrypt` propagiert den Fehler. Die Auswahl ist feature-flag-abhängig. Der Clients-Default für `PM34500_StrictCipherDecryption` ist `FALSE`; konkrete Serverflags wurden hier nicht live ermittelt. Auch strenge Entschlüsselung allein reicht nicht, wenn anschließend das ganze fehlerhafte Item aus der Erfolgsliste verschwindet. [SDK-Login][sdklogin], [SDK-Auswahl][sdkget], [Feature-Flags][flags]

**`list` gegen `export` zu vergleichen ist nützlich, aber nicht unabhängig.** Beide können dieselben Items oder Felder verlieren. Ein leeres Passkey-Array könnte „kein Passkey vorhanden“ oder „Entschlüsselung fehlgeschlagen“ bedeuten. Ein Validator kann fehlendes Schlüsselmaterial nur erkennen, wenn er unabhängig weiß, dass ein Passkey erwartet wurde.

Für eine belastbare Freigabe braucht es deshalb entweder eine geprüfte CLI mit strikt propagierten Item-/Feldfehlern und Inventarbericht oder einen eng versionierten Abgleich mit dem verschlüsselten Ausgangsbestand. Das lokale CLI-Profil ist dafür eine mögliche, aber interne und veränderliche Quelle. Legacy-Felder erlauben eher Strukturzählungen; neue verschlüsselte Blob-Formate dürfen nicht als gleichermaßen prüfbar angenommen werden. Bei nicht prüfbarem Format: Lauf als nicht verifiziert beenden. Keine eigene Kryptografie als beiläufige MVP-Erweiterung.

**Konsistenz:** Vollsync → Inventar A → Export/Downloads → erneuter Vollsync → Inventar B. Änderungen an IDs, Revisionen, Anhängen und Organisationen führen zu begrenztem Retry beziehungsweise Abbruch. Ein ruhiger Schreibzeitraum verbessert die Aussagekraft. Gleiche Inventare vor/nachher sind trotzdem kein transaktionaler Snapshot; mehrere Accounts und Organisations-API-Aufrufe haben unterschiedliche Zeitpunkte. Vorherige Generationen behalten und dieses Konsistenzniveau im Manifest benennen.

**Credentials und Interaktion:**

- Interaktives `bw login --raw` bietet E-Mail, Masterpasswort und TOTP-Abfrage; Authenticator ist Methode 0. Ein Terminal-/PTY-Adapter muss Prompts weiterreichen und stdout für den Session-Key getrennt halten. TOTP-Anmeldung ist etwas anderes als das Sichern gespeicherter Passkeys.
- Passwort, TOTP und Session nicht in Argumentlisten, Logs, Shell-History oder Manifest schreiben. `--session` existiert, ist wegen sichtbarer Prozessargumente kein bevorzugter Übergabeweg. `BW_SESSION` nur in der Umgebung des notwendigen Kindprozesses ist besser, aber keine Geheimnisgrenze gegenüber privilegierten lokalen Prozessen.
- Ein eigener `BITWARDENCLI_APPDATA_DIR` je Server+Account; pro Profil nur ein aktiver Lauf. Session-State, Cache und Tokens als sensibel behandeln. Persönliche vorhandene CLI-Profile nicht wiederverwenden.
- Zum Ende `bw lock` auch bei Abbruch; für Wegwerfprofile zusätzlich kontrollierter Logout und Bereinigung. Klartext-Staging auf geschütztem/verschlüsseltem Datenträger, POSIX 0700/0600 beziehungsweise passende Windows-ACL. Sichere Löschung auf SSDs nicht versprechen.
- Bot-/New-Device-Prüfungen können zusätzliche Interaktion verlangen; SSO ohne unterstützten Entschlüsselungsweg ist kein zugesicherter MVP-Fall. Keine automatische Ausweichanmeldung mit API-Key, die das gewollte TOTP-Verfahren verändert.

Quellen: [CLI-Dokumentation][clidoc], [Login-Implementierung][logincommand], [Session-Storage][session].

## 5. Plattformen, Regionen und Vaultwarden

Offizielle 2026.10.1-Bundles existieren für **Windows x64, Linux x64/ARM64 und macOS x64/ARM64**, jeweils OSS und Non-OSS. Windows ARM64-native Unterstützung ist durch diese Release-Assets nicht belegt. „Native portable“ bedeutet hier ein plattformspezifisch paketierter Node-basierter Prozess ohne Docker/WSL-Zwang, keine systemnahe Rust-/C-Binärdatei. Tatsächlich ausgeführt wurde nur macOS ARM64. Mindest-OS-Versionen, glibc/musl, Proxy, Zertifikate und Signierung sind vor einer konkreten Supportzusage zu testen. [Release][release], [Buildziele][package]

EU: `bw config server https://vault.bitwarden.eu`; US: passende US-Konfiguration, typischerweise `https://vault.bitwarden.com`; Custom/Vaultwarden: eigene Basis-URL. Separate API-/Identity-Endpunkte sind konfigurierbar. Kein Serverwechsel im gemeinsam genutzten angemeldeten Profil. [CLI-Konfiguration][clidoc]

Vaultwarden 1.37.3 wurde mit CLI 2026.9.0 wegen fehlendem `POST /api/accounts/key-management/user-key-id` als inkompatibel gemeldet. Release 1.37.4 enthält die zugehörige Änderung; der Endpoint ist am oben genannten Commit in `accounts.rs` vorhanden. **Damit ist dieser konkrete Bruch behoben, aber CLI 2026.10.1 + Vaultwarden 1.37.4 ist hier nicht Ende-zu-Ende zertifiziert.** Dieses Paar ist der erste Live-Testkandidat; 1.37.3 ein gezielter negativer Kompatibilitätstest. [Issue #7750][vwissue], [1.37.4][vwrelease], [Endpoint][vwendpoint]

## 6. Pinning und Pflege

Pinning muss CLI-Version **und Asset-Hash und Plattform/Buildvariante** umfassen. Im Manifest zusätzlich Clients-SHA, SDK-Version, beobachtete Serverversion, relevante verfügbare Flags, Schema-/Validatorversion und Prüfergebnisse speichern. Ein SDK-Quellcode-SHA aus npm-Metadaten ist eine Zuordnung, keine selbst durchgeführte Build-Attestierung.

Keine automatische Verwendung von `latest`. Neue CLI-/Server-Paare zuerst mit Golden Fixtures und Fehlerfällen prüfen; unbekannte Kombinationen zunächst sperren oder ausdrücklich als experimentell ausweisen. Nicht unbegrenzt bei einem alten Release bleiben: Sicherheitsupdates brauchen einen beschleunigten Qualifizierungsweg.

Dass Lieferkettenkontrollen relevant sind, ist konkret belegt: Bitwarden veröffentlichte eine Erklärung zu einer kurzzeitig kompromittierten npm-Auslieferung von `@bitwarden/cli@2026.4.0` im April 2026. Das ist keine Aussage, dass der hier verwendete Release kompromittiert sei. [Bitwarden-Erklärung][incident]

Vorhandene Upstream-Tests behandeln JSON, Policy-Filter, übersprungene Anhänge sowie einfache doppelte Namen. Die hier reproduzierten kollidierenden bereinigten Namen sind dadurch offensichtlich nicht verhindert. CI führt Tests mit Coverage aus. Upstream-Testkonfiguration ersetzt keine eigene Backup-Vertragsprüfung. [Exporttests][exporttests], [CI][ci]

Diff-Prüfung vor Updates: Exportmapper, `getAllDecrypted`/SDK-Decryptor, Login/Sync, Attachment-Download, CLI-Optionen, Rechtefilter und importierbare JSON-Struktur. Neue Itemtypen oder Blob-Formate sind Vertragsänderungen, auch wenn der CLI-Aufruf unverändert bleibt. Das interne SDK direkt als öffentliche stabile API einzuplanen ist ungünstig; die Paketbeschreibung schließt Support ausdrücklich aus. [SDK-Metadaten][sdkmeta]

## 7. Durchgeführte Tests und konkrete Freigabematrix

**Ausgeführt: acht isolierte Mock-Prüfungen, alle erwarteten Ergebnisse bestätigt.** Dabei bedeuten bestandene Fehlerreproduktionen, dass der Fehler nachgewiesen wurde, nicht dass die CLI korrekt arbeitet.

| Mock-Prüfung | Ergebnis |
|---|---|
| 13 Passkey-Felder, Zähler 6, eigenes Feld, Signatur nach JSON-Export | Erhalten/verifiziert |
| JSON versus ZIP-`data.json`, erfolgreiche Anhangsbytes | Gleich / erhalten |
| Simulierter Downloadfehler | ZIP mit Skip-Zähler; CLI-Speicherpfad trotzdem erfolgreich |
| `a/b` versus `a_b` | Zwei erwartete Dateien → eine Datei |
| `x.txt`, `x.txt`, `x_1.txt` | Drei erwartete Dateien → zwei Dateien |
| Policy-gefiltertes Item | JSON enthält es nicht, ZIP enthält seinen Anhang |
| ZIP plus Passwort | Routing zu verschlüsseltem JSON |
| Sync-Service liefert `false` | Sync-Befehl meldet Erfolg |

**Binary-Smoke-Tests:** Version und Status erfolgreich; Login ohne erforderliche Angaben sowie Sync/Export/Attachment-Abruf ohne Anmeldung mit Exit 1; derselbe Sync mit `BW_CLEANEXIT=false` mit Exit 0. Ausführung im frischen eigenen Profil, ohne Konto. Die Umgebung war dabei offline/sandboxbeschränkt; die Serverconfig-DNS-Fehler sind keine Aussage über die Erreichbarkeit der echten Bitwarden-Dienste. Kein authentifizierter Download wurde damit getestet.

**Noch auszuführen, vor Produktfreigabe verbindlich:**

| Ebene | Konkrete Fälle | Erfolgskriterium |
|---|---|---|
| Betriebssysteme | Windows x64, Linux x64/ARM64, macOS x64/ARM64; Unicode-Pfade, lange Pfade, case-insensitive Dateisysteme | Dieselben IDs/Felder/Hashes, korrekter Abbruch und geschützte Dateien |
| Server | Bitwarden US + EU; gepinnter offizieller Self-host; Vaultwarden 1.37.4; negatives Paar 1.37.3/CLI 2026.9.0 | Serveridentität geprüft; unsupported Kombination scheitert sichtbar |
| Anmeldung | Synthetische Konten, PBKDF2 und Argon2id, gültige/falsche/abgelaufene TOTP, Abbruch, Bot-/Device-Challenge | Keine Geheimnisse im Log; kein Backup bei unvollständiger Anmeldung |
| Session | Gesperrt, abgelaufen, falscher Session-Key, parallele Profile, gleichzeitiger Lauf desselben Profils | Kein Cross-Account-Mix; Profil-Lock verhindert Rennen |
| Passkey normal | Lokaler WebAuthn-Test-RP, registrierter P-256-Testpasskey mit Zähler 0, 16-/32-Byte-ID, optionale Metadaten, mehrere Fixture-Credentials | Alle bekannten Felder erhalten; nach manuellem Testimport echte RP-Anmeldung möglich |
| Passkey historisch | Synthetischer Zähler >0, RP-Zustand vor/nach weiterer Anmeldung | Zähler bleibt exakt erhalten; veralteter Zähler wird als Wiederverwendungsrisiko demonstriert |
| Passkey beschädigt | Manipuliertes verschlüsseltes `keyValue`, Counter, Credential-Array; strict-Flag an/aus | Kein Erfolg mit verschwundenem Passkey oder verschwundenem Item |
| Anhänge | Leerdatei, Binärdatei mit NUL, Unicode, mehrere gleiche Namen, `/`, `\\`, `..`, reservierte Windows-Namen, große Datei | ID-basierte Ablage, exakte Fixture-SHA-256, keine Kollision/Pfadflucht |
| Downloadfehler | 403/404/429/500, Timeout, Verbindungsabbruch, verkürzter Ciphertext, ungültiger MAC, abgelaufene URL, Legacy-Anhang ohne eigenen Key | Kein finales Archiv; begrenzte Retries nur für geeignete temporäre Fehler |
| Organisationen | Owner/Admin, normales Mitglied, verwaltete Collection, verborgene Passwörter, fehlende Collection-Zuweisung, widerrufene Rechte | Vollständiger deklarierter Scope oder klarer Abbruch; keine Teilmenge als „alles“ |
| Bestandskonsistenz | Item/Passkey/Anhang während Export ändern oder löschen, Item in Organisation verschieben | Änderung erkannt; Retry/Abbruch, alte Generation bleibt erhalten |
| Beschädigter Bestand | Unentschlüsselbares Item, fehlender Org-Key, Partial/PAM-Item, unbekannter Typ/Blob | Unabhängiges Inventar erkennt Auslassung oder Lauf bleibt unverifiziert |
| Archiv | Datenträger voll, fehlende Rechte, Prozessabbruch in jedem Schritt, falsches Archivpasswort, manipuliertes Archiv | Keine Veröffentlichung/Retention bei Fehler; externe Entschlüsselung + Hashprüfung erfolgreich |

Live-Tests verwenden ausschließlich neue synthetische Konten und einen lokalen Test-RP. Fixtures legen erwartete Item-/Passkey-/Attachment-IDs und Hashes **vor** dem Export fest. Ein manueller Import in einen zweiten Testtresor ist eine Qualitätsprüfung, keine Implementierung eines automatischen Restore-Features. Die erste vollständige Live-Prüfung sollte genau den schwierigen Fall „Passkey + zwei kollidierende Attachment-Namen + Organisationsitem“ enthalten.

## 8. Schlanker MVP-Vorschlag und Freigabebedingung

**Ein kleiner plattformübergreifender Orchestrator, ein gepinntes `bw`, ein Standardarchiv plus `age`.** Eine Go-Implementierung wäre ein plausibler späterer Kandidat; hier wurde nichts davon implementiert.

1. **Profilverwaltung:** expliziter Server+Account, eigenes CLI-Datenverzeichnis, Prozess-Lock und bereinigte Umgebung. Interaktive Anmeldung/Entsperrung über Terminal; keine eigene Authentifizierung oder Kryptografie.
2. **Erfassung:** erzwungener Sync, unabhängiges erwartetes Inventar, nativer persönlicher JSON-Export und ausdrücklich ausgewählte Organisations-JSONs. Anhänge pro Item-ID/Attachment-ID herunterladen. Originalnamen nur als Metadaten speichern.
3. **Validierung:** erwartete gegen erhaltene IDs und alle bekannten Passkey-Felder prüfen; Schlüsselmaterial syntaktisch prüfen, keine privaten Schlüssel protokollieren. Anzahl allein reicht nicht. Unbekannte Entschlüsselungs-/Schemafälle blockieren. Anhangs-SHA-256 dient nach Erfassung der Integrität; er beweist ohne unabhängigen Sollwert nicht, dass die richtige Quelldatei geliefert wurde. Servergrößen können Ciphertextgrößen sein, nicht blind mit Klartextlängen vergleichen.
4. **Verpackung:** unveränderte native JSON-Dateien, ID-basierte Attachment-Dateien, Manifest und kurze manuelle Wiederherstellungsanleitung in TAR, dann `age`-Verschlüsselung. Manifest und Originalnamen liegen innerhalb der Verschlüsselung. `age` ist für Windows/Linux/macOS verfügbar; Format und unabhängige Implementierungen sind öffentlich. Empfängerschlüssel oder interaktiv eingegebene Archivpassphrase, getrennt vom Tresorpasswort. [age-Projekt][age]
5. **Veröffentlichung:** neues temporäres Archiv auf demselben Zieldateisystem, erfolgreiche Archivierung/Verschlüsselung und Inhaltsprüfung sicherstellen, dann atomar unter neuem Generationsnamen veröffentlichen. Retention erst danach. Bei Empfängerverschlüsselung ohne verfügbaren privaten Schlüssel ist eine sofortige Entschlüsselungsprüfung nicht möglich: periodische externe Wiederherstellungsprüfung bleibt Pflicht.

Beispielinhalt nach Standard-Entschlüsselung und Entpacken:

```text
manifest.json
accounts/<profil-id>/personal.json
accounts/<profil-id>/organizations/<org-id>.json
accounts/<profil-id>/attachments/<item-id>/<attachment-id>.bin
README-Wiederherstellung.txt
```

Das Manifest verknüpft IDs, ursprüngliche Dateinamen, Scope, Zeitfenster, Hashes, Versionen und Prüfstatus. JSON lässt sich über einen unterstützten offiziellen Importweg einlesen; Anhänge bleiben normale Dateien mit eindeutiger Zuordnung für manuelle Wiederherstellung. Das eigene Gesamtarchiv ist kein direkt importierbares Bitwarden-ZIP. KDBX kann später als zusätzliche abgeleitete Notfallkopie hinzukommen und ersetzt diesen nativen Kern nicht.

**Konkrete Freigabeschwelle:** GO für einen begrenzten Prototyp mit synthetischen Testkonten. Produktives GO erst nach (a) nachgewiesener Erkennung ausgelassener Items und Passkeys, (b) vollständiger Attachment-Inventarisierung auch im zugesagten Organisationsscope, (c) bestandenen Live-Roundtrips der unterstützten Server-/CLI-Paare und (d) extern lesbarem Archiv. Wenn das Projekt ausschließlich unveränderte öffentliche `bw`-Befehle nutzen und dennoch jederzeit vollständige Sicherung garantieren soll, bleibt die Entscheidung derzeit **NO-GO**.

[release]: https://github.com/bitwarden/clients/releases/tag/cli-v2026.10.1
[package]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/package.json
[package-root]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/package.json#L188
[sdkmeta]: https://registry.npmjs.org/@bitwarden/sdk-internal/0.2.0-main.1126
[fido]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/fido2-credential.export.ts#L118
[loginexport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/login.export.ts#L78
[cipherexport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/cipher.export.ts#L225
[fieldexport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/field.export.ts
[individual]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/individual-vault-export.service.ts#L95
[orgexport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/org-vault-export.service.ts#L106
[exportcmd]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/tools/export.command.ts#L103
[exportservice]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/vault-export.service.ts
[get]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/commands/get.command.ts#L345
[list]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/commands/list.command.ts#L300
[cipherresponse]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/vault/models/cipher.response.ts
[orgapi]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/api/default-vault-export-api.service.ts#L16
[vaultprogram]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/vault.program.ts#L529
[baseprogram]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/base-program.ts#L24
[logincommand]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/auth/commands/login.command.ts#L402
[synccommand]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/vault/sync.command.ts#L15
[syncservice]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/platform/sync/default-sync.service.ts#L130
[download]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/commands/download.command.ts#L36
[utils]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/utils.ts#L100
[cipherservice]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/vault/services/cipher.service.ts#L375
[sdkbridge]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/vault/services/cipher-sdk.service.ts#L460
[sdklogin]: https://github.com/bitwarden/sdk-internal/blob/4a9eb239fa7f63132ad7a32cce9aeb5fae252243/crates/bitwarden-vault/src/cipher/login.rs#L139
[sdkget]: https://github.com/bitwarden/sdk-internal/blob/4a9eb239fa7f63132ad7a32cce9aeb5fae252243/crates/bitwarden-vault/src/cipher/cipher_client/get.rs#L87
[flags]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/enums/feature-flag.enum.ts#L177
[session]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/platform/services/node-env-secure-storage.service.ts#L97
[importer]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/importer/src/importers/bitwarden/bitwarden-json-importer.ts#L52
[exporttests]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/individual-vault-export.service.spec.ts#L334
[ci]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/.github/workflows/test.yml#L119
[issuezip]: https://github.com/bitwarden/clients/issues/24335
[vwrelease]: https://github.com/dani-garcia/vaultwarden/releases/tag/1.37.4
[vwissue]: https://github.com/dani-garcia/vaultwarden/issues/7750
[vwendpoint]: https://github.com/dani-garcia/vaultwarden/blob/7c188967e7a467b19b2757bc3326f9cf3ccae327/src/api/core/accounts.rs#L1058
[clidoc]: https://bitwarden.com/help/cli/
[exportdoc]: https://bitwarden.com/help/export-your-data/
[notes]: https://bitwarden.com/de-de/help/releasenotes/
[incident]: https://community.bitwarden.com/t/bitwarden-statement-on-checkmarx-supply-chain-incident/96127
[age]: https://github.com/FiloSottile/age
