# Getting Started with Explore

1. Open the repository folder containing the top-level README in VS Code. Docker must be running and the Dev Containers extension installed.
2. Choose **Dev Containers: Reopen in Container**. The first build needs network access. The personal VS Code feature provides Claude Code.
3. In the container terminal, inside the repository, run `go version`, `bw --version`, `age --version`, `openspec --version` and `openspec list`.
4. Start `claude`. Input: `/openspec-explore Read docs/development/first-explore-prompt.md and work on the brief.`
5. Review the results and choose a scope. Only then run, for example, `/openspec-propose Plan a synthetic regression test for attachments with the same name but different IDs.` Review the proposal, design, spec delta and tasks under openspec/changes/. Run `openspec validate <change-name> --strict` before the next step.

OpenSpec was initialized with `openspec init --tools claude,agents`. The generated skills are checked in. Further clients can be connected as needed following the [official CLI reference](https://github.com/Fission-AI/OpenSpec/blob/v2.0.0/docs/cli.md).
