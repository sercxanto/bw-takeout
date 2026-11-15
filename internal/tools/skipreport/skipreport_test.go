package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stream is a recorded go test -json stream with one passing test and the
// skipped tests named in the arguments.
func stream(skips map[string]string, failed ...string) string {
	var b strings.Builder
	ev := func(action, pkg, test, out string) {
		b.WriteString(`{"Action":"` + action + `","Package":"` + pkg + `"`)
		if test != "" {
			b.WriteString(`,"Test":"` + test + `"`)
		}
		if out != "" {
			b.WriteString(`,"Output":"` + out + `"`)
		}
		b.WriteString("}\n")
	}
	ev("run", "m/p", "TestOK", "")
	ev("output", "m/p", "TestOK", "--- PASS: TestOK (0.00s)\\n")
	ev("pass", "m/p", "TestOK", "")
	for name, reason := range skips {
		ev("run", "m/p", name, "")
		ev("output", "m/p", name, "=== RUN   "+name+"\\n")
		ev("output", "m/p", name, "    x_test.go:1: "+reason+"\\n")
		ev("output", "m/p", name, "--- SKIP: "+name+" (0.00s)\\n")
		ev("skip", "m/p", name, "")
	}
	for _, name := range failed {
		ev("output", "m/p", name, "--- FAIL: "+name+" (0.00s)\\n")
		ev("fail", "m/p", name, "")
	}
	ev("output", "m/p", "", "ok  \\tm/p\\t0.1s\\n")
	if len(failed) > 0 {
		ev("fail", "m/p", "", "")
	} else {
		ev("pass", "m/p", "", "")
	}
	return b.String()
}

const list = `# comment

*       m/p TestAlways  # helper process
windows m/p TestWinOnly # no POSIX permissions
linux   m/p TestNever   # never skips
`

type result struct {
	status int
	stdout string
	stderr string
}

func runWith(t *testing.T, expected, goos, in string, env map[string]string) result {
	t.Helper()
	path := filepath.Join(t.TempDir(), "expected.txt")
	if err := os.WriteFile(path, []byte(expected), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	st := run([]string{"-expected", path, "-goos", goos}, strings.NewReader(in), &out, &errb,
		func(k string) string { return env[k] })
	return result{st, out.String(), errb.String()}
}

func TestEchoAndTable(t *testing.T) {
	in := stream(map[string]string{"TestAlways": "helper process"})
	r := runWith(t, list, "linux", in, nil)
	if r.status != 0 {
		t.Fatalf("status %d, stderr %q", r.status, r.stderr)
	}
	wantEcho := "--- PASS: TestOK (0.00s)\n=== RUN   TestAlways\n    x_test.go:1: helper process\n--- SKIP: TestAlways (0.00s)\nok  \tm/p\t0.1s\n"
	if !strings.HasPrefix(r.stdout, wantEcho) {
		t.Errorf("echo differs:\n%q\nwant prefix\n%q", r.stdout, wantEcho)
	}
	table := strings.TrimPrefix(r.stdout, wantEcho)
	want := "\n### Skipped tests on linux\n\n" +
		"| Package | Test | Expected | Reason |\n|---|---|---|---|\n" +
		"| m/p | TestAlways | yes | x_test.go:1: helper process |\n" +
		"\nExpected skips that did not skip (information):\n\n- m/p TestNever (linux)\n"
	if table != want {
		t.Errorf("table:\n%q\nwant\n%q", table, want)
	}
}

func TestExpectedSkipForThePlatform(t *testing.T) {
	r := runWith(t, list, "windows", stream(map[string]string{"TestWinOnly": "no permissions"}), nil)
	if r.status != 0 {
		t.Fatalf("status %d, stderr %q", r.status, r.stderr)
	}
}

func TestExpectedSkipForAnyPlatform(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		r := runWith(t, list, goos, stream(map[string]string{"TestAlways": "helper"}), nil)
		if r.status != 0 {
			t.Errorf("%s: status %d, stderr %q", goos, r.status, r.stderr)
		}
	}
}

func TestSubtestOfExpectedSkip(t *testing.T) {
	r := runWith(t, list, "linux", stream(map[string]string{"TestAlways/sub": "helper"}), nil)
	if r.status != 0 {
		t.Fatalf("status %d, stderr %q", r.status, r.stderr)
	}
}

func TestUnexpectedSkip(t *testing.T) {
	r := runWith(t, list, "linux", stream(map[string]string{"TestRecovery": "the age command is not in the search path"}), nil)
	if r.status != 1 {
		t.Fatalf("status %d, want 1", r.status)
	}
	if !strings.Contains(r.stdout, "| m/p | TestRecovery | **no** | x_test.go:1: the age command is not in the search path |") {
		t.Errorf("table does not name the skip with its reason:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "unexpected skip on linux: m/p TestRecovery") {
		t.Errorf("stderr: %q", r.stderr)
	}
}

func TestSkipExpectedOnlyOnAnotherPlatform(t *testing.T) {
	r := runWith(t, list, "linux", stream(map[string]string{"TestWinOnly": "no permissions"}), nil)
	if r.status != 1 {
		t.Fatalf("status %d, want 1", r.status)
	}
}

func TestFailedTest(t *testing.T) {
	r := runWith(t, list, "linux", stream(nil, "TestBroken"), nil)
	if r.status != 1 {
		t.Fatalf("status %d, want 1", r.status)
	}
	if !strings.Contains(r.stdout, "- m/p TestBroken") || !strings.Contains(r.stderr, "failed: m/p TestBroken") {
		t.Errorf("failure not reported:\nstdout %q\nstderr %q", r.stdout, r.stderr)
	}
}

func TestEntryThatDidNotSkip(t *testing.T) {
	r := runWith(t, list, "linux", stream(nil), nil)
	if r.status != 0 {
		t.Fatalf("status %d, stderr %q", r.status, r.stderr)
	}
	if !strings.Contains(r.stdout, "No test was skipped.") ||
		!strings.Contains(r.stdout, "- m/p TestAlways (*)") ||
		!strings.Contains(r.stdout, "- m/p TestNever (linux)") {
		t.Errorf("unused entries not listed:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout, "TestWinOnly") {
		t.Errorf("entry of another platform listed:\n%s", r.stdout)
	}
}

func TestLineWithoutReason(t *testing.T) {
	for _, bad := range []string{"linux m/p TestX\n", "linux m/p TestX #   \n"} {
		r := runWith(t, bad, "linux", stream(nil), nil)
		if r.status != 2 || !strings.Contains(r.stderr, "no reason") {
			t.Errorf("%q: status %d, stderr %q", bad, r.status, r.stderr)
		}
	}
}

func TestMalformedLine(t *testing.T) {
	for _, bad := range []string{"linux m/p # reason\n", "linux m/p TestX extra # reason\n", "plan9 m/p TestX # reason\n"} {
		r := runWith(t, bad, "linux", stream(nil), nil)
		if r.status != 2 || !strings.Contains(r.stderr, "line 1") {
			t.Errorf("%q: status %d, stderr %q", bad, r.status, r.stderr)
		}
	}
}

func TestMissingList(t *testing.T) {
	var out, errb bytes.Buffer
	st := run([]string{"-expected", filepath.Join(t.TempDir(), "none.txt")}, strings.NewReader(""), &out, &errb, func(string) string { return "" })
	if st != 2 {
		t.Fatalf("status %d, want 2", st)
	}
}

func TestSummaryFile(t *testing.T) {
	sum := filepath.Join(t.TempDir(), "summary.md")
	if err := os.WriteFile(sum, []byte("earlier\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := stream(map[string]string{"TestAlways": "helper"})
	r := runWith(t, list, "linux", in, map[string]string{"GITHUB_STEP_SUMMARY": sum})
	if r.status != 0 {
		t.Fatalf("status %d, stderr %q", r.status, r.stderr)
	}
	if strings.Contains(r.stdout, "### Skipped tests") {
		t.Errorf("table also on standard output")
	}
	got, err := os.ReadFile(sum)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "earlier\n") || !strings.Contains(string(got), "| m/p | TestAlways | yes |") {
		t.Errorf("summary file: %q", got)
	}
}

func TestNonJSONLinePassesThrough(t *testing.T) {
	r := runWith(t, list, "linux", "# m/p\nbuild message\n"+stream(nil), nil)
	if !strings.HasPrefix(r.stdout, "# m/p\nbuild message\n") {
		t.Errorf("stdout: %q", r.stdout)
	}
}
