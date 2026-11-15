# Contributions from Humans and Agents

Start with [getting started](docs/development/getting-started.md). Explore collects questions, evidence and alternatives. For a well-scoped change, Propose creates a change with motivation, spec delta, design and verifiable tasks. Review scope, risks and acceptance criteria together before implementing. After implementing, run the tests, compare the spec with the actual behavior and archive the reviewed change.

Keep changes small. Future Go code follows gofmt, explicit error handling and testable error paths for external processes and file operations. A PR names the change, its impact, the tests that were run, its CI run (see the [test strategy](docs/development/testing.md#skip-report)) and remaining gaps. For behavior changes, add matching regression tests according to the [test strategy](docs/development/testing.md).

Write all repository content in English, regardless of the language used in discussions.

Use synthetic data only. No credentials, sessions, plaintext exports or keys in arguments, logs, issues, fixtures or commits. Review diffs before sharing them.

Update the affected pages according to the [documentation overview](docs/README.md). An ADR is appropriate when a lasting decision is hard to reverse, changes integration boundaries or fixes an essential security assumption. Document context, alternatives and consequences. New decisions start as proposed. Acceptance requires an explicit project decision. Change-specific details belong in the design.
