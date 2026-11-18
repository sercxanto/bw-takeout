package run_test

import (
	"os"
	"strings"
	"testing"

	"bw-takeout/internal/bwcli/bwclitest"
	ct "bw-takeout/internal/capture/capturetest"
)

// leaks returns every secret value that appears in the two output streams
// of the run.
func (r reported) leaks(f *ct.Fixture) []string {
	output := r.stdout + r.stderr
	var found []string
	for _, s := range append(f.Sensitive(), ct.Marker, stderrSentinel, fakeKey, ct.AttachmentFileName, "AGE-SECRET-KEY") {
		if strings.Contains(output, s) {
			found = append(found, s)
		}
	}
	return found
}

func TestSentinelsInTheVaultAndInBWOutput(t *testing.T) {
	failedVerdict := func(t *testing.T) *ct.Fixture { return ct.VariantByName(t, ct.MissingItem).Build(t) }
	for name, c := range map[string]struct {
		fixture func(t *testing.T) *ct.Fixture
		change  func(s *bwclitest.Scenario)
		signal  string
	}{
		"verified":       {fixture: func(t *testing.T) *ct.Fixture { return ct.Intact(t) }},
		"failed verdict": {fixture: failedVerdict},
		"failed download": {fixture: func(t *testing.T) *ct.Fixture { return ct.Intact(t) }, change: func(s *bwclitest.Scenario) {
			s.Commands[secondAttachment] = bwclitest.Behavior{Exit: 1, Stdout: ct.Marker + "-stdout-of-bw", Stderr: stderrSentinel}
		}},
		"failed logout": {fixture: func(t *testing.T) *ct.Fixture { return ct.Intact(t) }, change: func(s *bwclitest.Scenario) {
			s.Commands[cmdLogout] = bwclitest.Behavior{Exit: 1, Stdout: ct.Marker + "-stdout-of-bw", Stderr: stderrSentinel}
		}},
		"interrupted": {fixture: func(t *testing.T) *ct.Fixture { return ct.Intact(t) }, change: func(s *bwclitest.Scenario) {
			s.Commands[secondAttachment] = bwclitest.Behavior{Hang: true, Stderr: stderrSentinel}
		}, signal: secondAttachment},
	} {
		t.Run(name, func(t *testing.T) {
			f := c.fixture(t)
			s := scenarioOf(t, f)
			if c.change != nil {
				c.change(&s)
			}
			e := newEnv(t, s)
			if c.signal != "" {
				go func() {
					e.waitForCommand(c.signal)
					e.signals <- os.Interrupt
				}()
			}
			r := e.runReported()
			if len(f.Sensitive()) < 10 || !strings.Contains(r.stdout, "Logout:") {
				t.Fatalf("precondition: %d sentinels\n%s", len(f.Sensitive()), r.stdout)
			}
			if found := r.leaks(f); len(found) != 0 {
				t.Errorf("output contains %q\nstdout:\n%s\nstderr:\n%s", found, r.stdout, r.stderr)
			}
		})
	}
}

func TestControlCharactersInTheAccountAddress(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdStatus] = statusBehavior(fakeLastSync, fakeServer+"/\x1b[2J", "user\x1b]0;title\x07@example.test")
	e := newEnv(t, s)
	r := e.runReported()
	if r.o.Source == nil || !strings.Contains(r.o.Source.AccountEmail, "\x1b") {
		t.Fatalf("precondition: source %+v", r.o.Source)
	}
	if strings.ContainsAny(r.stdout+r.stderr, "\x1b\x07") {
		t.Errorf("control characters in the output\n%q", r.stdout)
	}
	r.want(t, "Account:     user?]0;title?@example.test (", "Synced from: "+fakeServer+"/?[2J\n")
}
