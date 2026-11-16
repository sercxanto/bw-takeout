package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The dev container has no YAML parser and the module has no YAML
// dependency, so these tests read the workflows under .github/workflows as
// plain text. They check the structure that the pins, the matrix and the
// path filters rely on. They do not replace a YAML parser: GitHub validates
// the files on the first run.

func readWorkflow(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../../.github/workflows/" + name)
	if err != nil {
		t.Fatal(err)
	}
	// A Windows checkout may convert line endings.
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// checkCommon checks what every workflow must hold and returns the number
// of its uses lines.
func checkCommon(t *testing.T, text string) int {
	t.Helper()
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

	pin := regexp.MustCompile(`uses: \S+@[0-9a-f]{40} # v\d+\.\d+\.\d+$`)
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "uses:") {
			n++
			if !pin.MatchString(strings.TrimSpace(line)) {
				t.Errorf("not pinned by full SHA with tag comment: %s", line)
			}
		}
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
	return n
}

// pathLists returns the path filter lists of the workflow, one per trigger
// that has one, as the lines under "paths:".
func pathLists(text string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "paths:" {
			continue
		}
		var list []string
		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if !strings.HasPrefix(trimmed, "- ") {
				break
			}
			list = append(list, trimmed)
		}
		out = append(out, strings.Join(list, "\n"))
	}
	return out
}

func TestWorkflowStructure(t *testing.T) {
	ci := readWorkflow(t, "ci.yml")
	checks := readWorkflow(t, "checks.yml")
	uses := 0
	t.Run("ci.yml", func(t *testing.T) { uses += checkCommon(t, ci) })
	t.Run("checks.yml", func(t *testing.T) { uses += checkCommon(t, checks) })
	if uses != 4 {
		t.Errorf("%d uses lines in both workflows, want 4", uses)
	}

	m := regexp.MustCompile(`(?m)^        os: \[([^\]]*)\]$`).FindStringSubmatch(ci)
	if m == nil {
		t.Fatal("matrix line not found in ci.yml")
	}
	var got []string
	for _, s := range strings.Split(m[1], ",") {
		got = append(got, strings.TrimSpace(s))
	}
	want := []string{"ubuntu-24.04", "macos-26", "windows-2025"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("matrix runners %v, want %v", got, want)
	}
}

// The test matrix runs on changes to Go code and its inputs. Both triggers
// use the same list, and the list names every input of the tests.
func TestTestPathsFilter(t *testing.T) {
	lists := pathLists(readWorkflow(t, "ci.yml"))
	if len(lists) != 2 {
		t.Fatalf("%d path lists in ci.yml, want one under push and one under pull_request", len(lists))
	}
	if lists[0] != lists[1] {
		t.Errorf("path lists differ:\n%s\n---\n%s", lists[0], lists[1])
	}
	for _, need := range []string{`- "**.go"`, "- go.mod", "- go.sum", "- .github/workflows/ci.yml", "- .github/expected-skips.txt"} {
		if !strings.Contains(lists[0]+"\n", need+"\n") {
			t.Errorf("path list lacks %s", need)
		}
	}
}

// The checks that cover every file, among them the key-material search,
// run on every change, so checks.yml has no path filter.
func TestChecksRunOnEveryChange(t *testing.T) {
	text := readWorkflow(t, "checks.yml")
	if lists := pathLists(text); len(lists) != 0 || strings.Contains(text, "paths-ignore") {
		t.Error("checks.yml filters by path")
	}
	// The key-material step is found by its message. Naming one of its
	// patterns here would make the search find this file.
	for _, need := range []string{"gofmt -l .", "go mod tidy -diff", "git grep --untracked", "Key material found in the repository."} {
		if !strings.Contains(text, need) {
			t.Errorf("checks.yml lacks %q", need)
		}
	}
}
