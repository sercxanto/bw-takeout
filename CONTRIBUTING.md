# Beiträge von Menschen und Agenten

Beginne beim [Entwicklungseinstieg](docs/development/getting-started.md). In Explore werden Fragen, Belege und Alternativen gesammelt. Für eine abgegrenzte Änderung erstellt Propose einen Change mit Motivation, Spezifikationsdelta, Design und prüfbaren Tasks. Vor der Umsetzung Scope, Risiken und Abnahmekriterien gemeinsam prüfen. Nach der Umsetzung Tests ausführen, Spezifikation und tatsächliches Verhalten vergleichen und den geprüften Change archivieren.

Halte Änderungen klein. Für späteren Go-Code gelten gofmt, explizite Fehlerbehandlung und überprüfbare Fehlerpfade bei externen Prozessen und Dateioperationen. Ein PR nennt Change, Auswirkungen, ausgeführte Tests und verbleibende Lücken. Ergänze bei Verhaltensänderungen passende Regressionstests gemäß [Teststrategie](docs/development/testing.md).

Verwende ausschließlich synthetische Daten. Keine Zugangsdaten, Sitzungen, Klartextexporte oder Schlüssel in Argumenten, Logs, Issues, Fixtures oder Commits. Prüfe Diffs vor dem Teilen.

Aktualisiere die betroffenen Seiten gemäß [Dokumentationsübersicht](docs/README.md). Ein ADR ist sinnvoll, wenn eine dauerhafte Festlegung schwer umkehrbar ist, Integrationsgrenzen verändert oder eine wesentliche Sicherheitsannahme festschreibt. Dokumentiere Kontext, Alternativen und Folgen. Neue Entscheidungen beginnen als proposed. Die Annahme erfordert eine ausdrückliche Projektentscheidung. Change-lokale Details gehören ins Design.
