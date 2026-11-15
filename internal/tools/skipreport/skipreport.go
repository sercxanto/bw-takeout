// Command skipreport reads the output of go test -json from standard input,
// echoes the test output, lists every skipped test with its reason and fails
// when a test failed or a test was skipped that the list of expected skips
// does not allow for the platform. No other package imports it.
//
// Usage:
//
//	go test -count=1 -json ./... | go run ./internal/tools/skipreport
//
// The table goes to the file named by GITHUB_STEP_SUMMARY when that variable
// is set, and to standard output otherwise.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
)

const defaultExpectedFile = ".github/expected-skips.txt"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}

// run returns the exit status: 0 when all tests passed and every skip is
// expected, 1 when a test failed or a skip is unexpected, 2 on a usage or
// input error such as a malformed list of expected skips.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("skipreport", flag.ContinueOnError)
	fs.SetOutput(stderr)
	expectedPath := fs.String("expected", defaultExpectedFile, "file with the expected skips")
	goos := fs.String("goos", runtime.GOOS, "platform the tests ran on")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	f, err := os.Open(*expectedPath)
	if err != nil {
		fmt.Fprintf(stderr, "skipreport: %v\n", err)
		return 2
	}
	expected, err := parseExpected(f)
	f.Close()
	if err != nil {
		fmt.Fprintf(stderr, "skipreport: %s: %v\n", *expectedPath, err)
		return 2
	}

	rep, err := readStream(stdin, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "skipreport: reading test output: %v\n", err)
		return 2
	}
	rep.classify(expected, *goos)

	table := rep.render(*goos)
	if path := getenv("GITHUB_STEP_SUMMARY"); path != "" {
		sf, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			fmt.Fprintf(stderr, "skipreport: %v\n", err)
			return 2
		}
		_, werr := io.WriteString(sf, table)
		cerr := sf.Close()
		if err := errors.Join(werr, cerr); err != nil {
			fmt.Fprintf(stderr, "skipreport: %v\n", err)
			return 2
		}
	} else {
		io.WriteString(stdout, table)
	}

	if len(rep.failed) > 0 || rep.unexpected() > 0 {
		for _, k := range rep.failed {
			fmt.Fprintf(stderr, "skipreport: failed: %s\n", k)
		}
		for _, s := range rep.skips {
			if !s.allowed {
				fmt.Fprintf(stderr, "skipreport: unexpected skip on %s: %s %s: %s\n", *goos, s.pkg, s.test, s.reason)
			}
		}
		return 1
	}
	return 0
}

// expectedSkip is one line of the list of expected skips.
type expectedSkip struct {
	platform string
	pkg      string
	test     string
	reason   string
}

// covers reports whether the entry allows a skip of the test: the test
// itself, or a subtest of it.
func (e expectedSkip) covers(pkg, test, goos string) bool {
	if e.platform != "*" && e.platform != goos {
		return false
	}
	if e.pkg != pkg {
		return false
	}
	return test == e.test || strings.HasPrefix(test, e.test+"/")
}

// parseExpected reads the list: one entry per line with platform, package
// and test name separated by white space, then "#" and the reason. Empty
// lines and lines that start with "#" are ignored.
func parseExpected(r io.Reader) ([]expectedSkip, error) {
	var out []expectedSkip
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		head, reason, ok := strings.Cut(line, "#")
		if !ok || strings.TrimSpace(reason) == "" {
			return nil, fmt.Errorf("line %d: the entry has no reason after #", n)
		}
		fields := strings.Fields(head)
		if len(fields) != 3 {
			return nil, fmt.Errorf("line %d: want platform, package and test before #, got %d fields", n, len(fields))
		}
		switch fields[0] {
		case "linux", "darwin", "windows", "*":
		default:
			return nil, fmt.Errorf("line %d: unknown platform %q", n, fields[0])
		}
		out = append(out, expectedSkip{
			platform: fields[0],
			pkg:      fields[1],
			test:     fields[2],
			reason:   strings.TrimSpace(reason),
		})
	}
	return out, sc.Err()
}

// event is one record of go test -json.
type event struct {
	Action  string
	Package string
	Test    string
	Output  string
}

type key struct{ pkg, test string }

func (k key) String() string {
	if k.test == "" {
		return k.pkg
	}
	return k.pkg + " " + k.test
}

type skip struct {
	pkg, test string
	reason    string
	allowed   bool
	because   string // reason of the matching entry
}

type report struct {
	output  map[key][]string // output lines per test
	order   []key            // skipped tests in order of appearance
	failed  []key
	skips   []skip
	unused  []expectedSkip // entries that did not skip
	skipSet map[key]bool
}

// readStream copies the test output to w and collects skips and failures.
func readStream(r io.Reader, w io.Writer) (*report, error) {
	rep := &report{output: map[key][]string{}, skipSet: map[key]bool{}}
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			var ev event
			if jerr := json.Unmarshal([]byte(line), &ev); jerr != nil || ev.Action == "" {
				// Not a test event, for example a build message: pass it on.
				io.WriteString(w, line)
			} else {
				rep.add(ev, w)
			}
		}
		if err == io.EOF {
			return rep, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (rep *report) add(ev event, w io.Writer) {
	k := key{ev.Package, ev.Test}
	switch ev.Action {
	case "output":
		io.WriteString(w, ev.Output)
		if ev.Test != "" {
			rep.output[k] = append(rep.output[k], ev.Output)
		}
	case "skip":
		if ev.Test != "" && !rep.skipSet[k] {
			rep.skipSet[k] = true
			rep.order = append(rep.order, k)
		}
	case "fail":
		rep.failed = append(rep.failed, k)
	}
}

// reasonOf returns the output lines of a test without the markers that go
// test adds, joined by a space.
func reasonOf(lines []string) string {
	var parts []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "=== ") || strings.HasPrefix(t, "--- ") {
			continue
		}
		parts = append(parts, t)
	}
	if len(parts) == 0 {
		return "(no reason in the output)"
	}
	return strings.Join(parts, " ")
}

// classify marks each skip as allowed or not for goos and finds the entries
// that matched no skip.
func (rep *report) classify(expected []expectedSkip, goos string) {
	used := make([]bool, len(expected))
	for _, k := range rep.order {
		s := skip{pkg: k.pkg, test: k.test, reason: reasonOf(rep.output[k])}
		for i, e := range expected {
			if e.covers(k.pkg, k.test, goos) {
				s.allowed = true
				s.because = e.reason
				used[i] = true
				break
			}
		}
		rep.skips = append(rep.skips, s)
	}
	for i, e := range expected {
		if !used[i] && (e.platform == "*" || e.platform == goos) {
			rep.unused = append(rep.unused, e)
		}
	}
}

func (rep *report) unexpected() int {
	n := 0
	for _, s := range rep.skips {
		if !s.allowed {
			n++
		}
	}
	return n
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

// render returns the summary as a Markdown table.
func (rep *report) render(goos string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n### Skipped tests on %s\n\n", goos)
	if len(rep.skips) == 0 {
		b.WriteString("No test was skipped.\n")
	} else {
		b.WriteString("| Package | Test | Expected | Reason |\n|---|---|---|---|\n")
		for _, s := range rep.skips {
			status := "**no**"
			if s.allowed {
				status = "yes"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", cell(s.pkg), cell(s.test), status, cell(s.reason))
		}
	}
	if len(rep.unused) > 0 {
		b.WriteString("\nExpected skips that did not skip (information):\n\n")
		unused := append([]expectedSkip(nil), rep.unused...)
		sort.SliceStable(unused, func(i, j int) bool { return unused[i].test < unused[j].test })
		for _, e := range unused {
			fmt.Fprintf(&b, "- %s %s (%s)\n", e.pkg, e.test, e.platform)
		}
	}
	if len(rep.failed) > 0 {
		b.WriteString("\nFailed:\n\n")
		for _, k := range rep.failed {
			fmt.Fprintf(&b, "- %s\n", k)
		}
	}
	return b.String()
}
