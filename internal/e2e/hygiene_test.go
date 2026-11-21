//go:build vaultwarden && linux

package e2e

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// The output check of design D9. The tests record every password, address
// and session key they generate or receive. TestMain copies the test log
// on its way to standard output and searches it after the run, so a run
// whose log holds one of them fails. A session key of the product never
// reaches the test, so the log is also searched for anything of its form.

var (
	secretsMu sync.Mutex
	secrets   []secret
)

type secret struct{ kind, value string }

// keepOutOfLog records a value that must not appear in the test log.
func keepOutOfLog(kind, value string) {
	if value == "" {
		return
	}
	secretsMu.Lock()
	defer secretsMu.Unlock()
	secrets = append(secrets, secret{kind, value})
}

// sessionKeyForm is base64 text of 88 characters that decodes to 64
// bytes, the form bwcli checks.
var sessionKeyForm = regexp.MustCompile(`[A-Za-z0-9+/]{86}==`)

// teeStdout replaces os.Stdout with a pipe whose content goes to the real
// standard output and into memory. The returned function restores
// os.Stdout and returns what passed.
func teeStdout() (func() []byte, error) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		io.Copy(io.MultiWriter(orig, &buf), r)
		close(done)
	}()
	os.Stdout = w
	return func() []byte {
		w.Close()
		<-done
		r.Close()
		os.Stdout = orig
		return buf.Bytes()
	}, nil
}

// logFindings names the kinds of recorded values found in the log, and a
// text of the session key form. It never repeats a value.
func logFindings(log []byte) []string {
	text := string(log)
	secretsMu.Lock()
	defer secretsMu.Unlock()
	seen := map[string]bool{}
	var found []string
	for _, s := range secrets {
		if !seen[s.kind] && strings.Contains(text, s.value) {
			seen[s.kind] = true
			found = append(found, s.kind)
		}
	}
	if sessionKeyForm.MatchString(text) {
		found = append(found, "text of the session key form")
	}
	return found
}

// checkLog reports the findings on standard error and returns whether the
// log is clean.
func checkLog(log []byte) bool {
	found := logFindings(log)
	if len(found) == 0 {
		fmt.Fprintf(os.Stderr, "log check: %d recorded values, none in %d bytes of test output\n", countSecrets(), len(log))
		return true
	}
	fmt.Fprintf(os.Stderr, "log check: the test output holds: %s\n", strings.Join(found, ", "))
	return false
}

func countSecrets() int {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	return len(secrets)
}

func TestLogCheckFindsRecordedValues(t *testing.T) {
	value := newUUID(t)
	keepOutOfLog("unit value", value)
	key := strings.Repeat("A", 86) + "=="
	found := logFindings([]byte("line one\nsomething " + value + " and " + key + "\n"))
	if strings.Join(found, ",") != "unit value,text of the session key form" {
		t.Fatalf("findings %v", found)
	}
	for _, f := range found {
		if strings.Contains(f, value) {
			t.Fatal("a finding repeats the value")
		}
	}
	if got := logFindings([]byte("clean log\n")); len(got) != 0 {
		t.Fatalf("findings in a clean log: %v", got)
	}
}
