# Mit Explore beginnen

1. Öffne den Repository-Ordner mit dem obersten README in VS Code. Docker muss laufen und die Erweiterung Dev Containers installiert sein.
2. Wähle **Dev Containers: Reopen in Container**. Der erste Build benötigt Netzwerk. Das persönliche VS-Code-Feature stellt Claude Code bereit.
3. Im Container-Terminal im Repository: `go version`, `bw --version`, `age --version`, `openspec --version` und `openspec list` ausführen.
4. Starte `claude`. Eingabe: `/openspec-explore Lies docs/development/first-explore-prompt.md und bearbeite den Auftrag.`
5. Ergebnisse prüfen und Scope wählen. Erst danach beispielsweise `/openspec-propose Plane einen synthetischen Regressionstest für gleichnamige Anhänge mit verschiedenen IDs.` Proposal, Design, Spec-Delta und Tasks unter openspec/changes/ prüfen. Vor dem nächsten Schritt `openspec validate <change-name> --strict` ausführen.

OpenSpec wurde mit `openspec init --tools claude,agents` initialisiert. Die generierten Skills sind eingecheckt. Weitere Clients lassen sich bei Bedarf nach der [offiziellen CLI-Referenz](https://github.com/Fission-AI/OpenSpec/blob/v2.0.0/docs/cli.md) anbinden.
