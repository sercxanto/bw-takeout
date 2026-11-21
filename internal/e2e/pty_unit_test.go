//go:build vaultwarden && linux

package e2e

import (
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

var childAnswers = map[string]string{"Email address": childEmail, "Master password": childPassword}

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

func TestUnknownPromptFails(t *testing.T) {
	_, err := runWithPrompts(childCommand(t, "unknown"), childAnswers, 30*time.Second)
	if err == nil {
		t.Fatal("an unknown prompt did not fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Two-step login code") || !strings.Contains(msg, "Email address") {
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
