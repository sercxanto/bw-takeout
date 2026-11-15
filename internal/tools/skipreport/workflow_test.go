package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The dev container has no YAML parser and the module has no YAML
// dependency, so this test reads .github/workflows/ci.yml as plain text. It
// checks the structure that the pins and the matrix rely on. It does not
// replace a YAML parser: GitHub validates the file on the first push.
func TestWorkflowStructure(t *testing.T) {
	raw, err := os.ReadFile("../../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "\t") {
		t.Error("the file contains a tab")
	}
	prev := 0
	for i, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent%2 != 0 {
			t.Errorf("line %d: odd indentation %d", i+1, indent)
		}
		if indent > prev+2 {
			t.Errorf("line %d: indentation jumps from %d to %d", i+1, prev, indent)
		}
		prev = indent
	}

	m := regexp.MustCompile(`(?m)^        os: \[([^\]]*)\]$`).FindStringSubmatch(text)
	if m == nil {
		t.Fatal("matrix line not found")
	}
	var got []string
	for _, s := range strings.Split(m[1], ",") {
		got = append(got, strings.TrimSpace(s))
	}
	want := []string{"ubuntu-24.04", "macos-26", "windows-2025"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("matrix runners %v, want %v", got, want)
	}

	pin := regexp.MustCompile(`uses: \S+@[0-9a-f]{40} # v\d+\.\d+\.\d+$`)
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "uses:") {
			n++
			if !pin.MatchString(strings.TrimSpace(line)) {
				t.Errorf("not pinned by full SHA with tag comment: %s", line)
			}
		}
	}
	if n != 4 {
		t.Errorf("%d uses lines, want 4", n)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "runs-on:") && !strings.Contains(line, "matrix.os") && !regexp.MustCompile(`-\d+\.\d+$`).MatchString(line) {
			t.Errorf("runner without versioned label: %s", line)
		}
	}
	for _, need := range []string{"permissions:\n  contents: read\n", "persist-credentials: false", "cache: false"} {
		if !strings.Contains(text, need) {
			t.Errorf("missing %q", need)
		}
	}
	if strings.Contains(text, "secrets.") || strings.Contains(text, "pull_request_target") {
		t.Error("the workflow names a secret or pull_request_target")
	}
}
