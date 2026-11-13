# Anforderungen

- Linux, macOS und Windows sind Zielplattformen. Der erste Prototyp bearbeitet ein Konto interaktiv mit TOTP. Mehrere Konten, Server und Organisationen gehören zum Ausbauziel.
- Persönliche Tresordaten, Passkeys einschließlich Schlüsselmaterial und Anhänge gehören ins Archiv. Server, Konto und Organisationsbereich müssen eindeutig zugeordnet werden können.
- Jede Sicherung erhält ein neues Ziel. Auch bei konkurrierenden Läufen darf keine ältere Sicherung überschrieben werden. Abbrüche dürfen kein scheinbar erfolgreiches Ergebnis hinterlassen.
- Exportumfang, Fehler und nicht prüfbare Bereiche müssen erkennbar bleiben. Eine absolute Vollständigkeitszusage ist mit der vorgesehenen Quelle nicht möglich.
- Offline-Lesbarkeit ist mit getrennt aufbewahrtem Schlüssel und synthetischem Bestand zu erproben.
