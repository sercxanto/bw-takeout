//go:build vaultwarden && linux

package e2e

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	childOnce sync.Once
	childPath string
	childErr  error
)

// buildPromptChild builds the child from testdata once, inside the module,
// so that it can use golang.org/x/term from go.mod. No source file is
// written during the run (review of 21 November 2026, R3).
func buildPromptChild(t *testing.T) string {
	t.Helper()
	childOnce.Do(func() {
		// The directory of the built bw-takeout, which TestMain removes.
		childPath = filepath.Join(filepath.Dir(takeout), "promptchild")
		build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-tags", "vaultwarden", "-o", childPath, "./testdata/promptchild")
		if b, err := build.CombinedOutput(); err != nil {
			childErr = err
			t.Logf("%s", b)
		}
	})
	if childErr != nil {
		t.Fatalf("build the prompt child: %v", childErr)
	}
	return childPath
}

const (
	childEmail    = "e2e-child@example.invalid"
	childPassword = "child-password-not-in-output"
)

func childCommand(t *testing.T, mode string) *exec.Cmd {
	cmd := exec.Command(buildPromptChild(t), mode)
	cmd.Env = []string{"WANT_EMAIL=" + childEmail, "WANT_PASSWORD=" + childPassword}
	return cmd
}

var childAnswers = map[string]answerFunc{"Email address": fixed(childEmail), "Master password": fixed(childPassword)}

func TestPromptsAnswered(t *testing.T) {
	run, err := runWithPrompts(childCommand(t, "known"), childAnswers, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode != 0 || strings.TrimSpace(string(run.Stdout)) != "true true" {
		t.Fatalf("exit %d, stdout %q", run.ExitCode, run.Stdout)
	}
	if strings.Join(run.Prompts, ",") != "Email address,Master password" {
		t.Fatalf("prompts %v", run.Prompts)
	}
}

const childCode = "424242"

// TestCodeAnswerComputedWhenPromptAppears checks design D3: the answer
// function of the third prompt runs once, only after the first two prompts
// were answered, and its text arrives in the child.
func TestCodeAnswerComputedWhenPromptAppears(t *testing.T) {
	var order []string
	note := func(name string, a answerFunc) answerFunc {
		return func() (string, error) {
			order = append(order, name)
			return a()
		}
	}
	answers := map[string]answerFunc{
		"Email address":       note("email", fixed(childEmail)),
		"Master password":     note("password", fixed(childPassword)),
		"Two-step login code": note("code", fixed(childCode)),
	}
	cmd := childCommand(t, "code")
	cmd.Env = append(cmd.Env, "WANT_CODE="+childCode)
	run, err := runWithPrompts(cmd, answers, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode != 0 || strings.TrimSpace(string(run.Stdout)) != "true true true" {
		t.Fatalf("exit %d, stdout %q", run.ExitCode, run.Stdout)
	}
	if got := strings.Join(order, ","); got != "email,password,code" {
		t.Fatalf("answer functions ran in the order %q, want each once: email,password,code", got)
	}
	if got := strings.Join(run.Prompts, ","); got != "Email address,Master password,Two-step login code" {
		t.Fatalf("prompts %v", run.Prompts)
	}
}

// TestFailedAnswerNamesOnlyThePrompt checks that an error of an answer
// function fails the run and that the message holds the prompt but neither
// the error text nor any answer.
func TestFailedAnswerNamesOnlyThePrompt(t *testing.T) {
	answers := map[string]answerFunc{
		"Email address":       fixed(childEmail),
		"Master password":     fixed(childPassword),
		"Two-step login code": func() (string, error) { return "", errors.New("secret-text-of-the-error") },
	}
	_, err := runWithPrompts(childCommand(t, "code"), answers, 30*time.Second)
	if err == nil {
		t.Fatal("a failing answer function did not fail the run")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Two-step login code") {
		t.Fatalf("message does not name the prompt: %s", msg)
	}
	for _, secret := range []string{childEmail, childPassword, "secret-text-of-the-error"} {
		if strings.Contains(msg, secret) {
			t.Fatalf("the message holds %q", secret)
		}
	}
}

func TestUnknownPromptFails(t *testing.T) {
	_, err := runWithPrompts(childCommand(t, "unknown"), childAnswers, 30*time.Second)
	if err == nil {
		t.Fatal("an unknown prompt did not fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Security question") || !strings.Contains(msg, "Email address") {
		t.Fatalf("message does not name the prompts: %s", msg)
	}
	if strings.Contains(msg, childEmail) || strings.Contains(msg, childPassword) {
		t.Fatal("the message holds an answer")
	}
}

func TestPromptTimeLimitFails(t *testing.T) {
	start := time.Now()
	_, err := runWithPrompts(childCommand(t, "hang"), childAnswers, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "no exit within") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 15*time.Second {
		t.Fatalf("the limit took %v", time.Since(start))
	}
	msg := err.Error()
	if !strings.Contains(msg, "Email address") {
		t.Fatalf("message does not name the prompt: %s", msg)
	}
	if strings.Contains(msg, childEmail) || strings.Contains(msg, childPassword) {
		t.Fatal("the message holds an answer")
	}
}
