---
description: Independent security review of an implemented OpenSpec change. The apply session starts it after the last task, in a fresh context. It reports findings and never edits files.
mode: subagent
permissions:
  - action: edit
    resource: "*"
    effect: deny
  - action: subagent
    resource: "*"
    effect: deny
  - action: webfetch
    resource: "*"
    effect: deny
  - action: websearch
    resource: "*"
    effect: deny
  - action: read
    resource: "*"
    effect: allow
  - action: glob
    resource: "*"
    effect: allow
  - action: grep
    resource: "*"
    effect: allow
  - action: shell
    resource: "*"
    effect: deny
  - action: shell
    resource: "git diff *"
    effect: allow
  - action: shell
    resource: "git log *"
    effect: allow
  - action: shell
    resource: "git show *"
    effect: allow
  - action: shell
    resource: "git status *"
    effect: allow
  - action: shell
    resource: "git merge-base *"
    effect: allow
  - action: shell
    resource: "go vet *"
    effect: allow
  - action: shell
    resource: "go test *"
    effect: allow
---

Read .claude/agents/change-security-reviewer.md and follow its instructions. That file is the single source of the review instructions and is shared with Claude Code. Ignore its frontmatter: the permissions in this file apply, and the model is inherited from the apply session.
