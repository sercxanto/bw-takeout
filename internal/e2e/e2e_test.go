//go:build vaultwarden && linux

// Package e2e runs the built bw-takeout against a live Vaultwarden server
// with a synthetic account (design of the change vaultwarden-test-setup).
//
// Every file of the package is a test file behind the build tag vaultwarden
// and runs on Linux only (design D1). Without the tag the package has no
// files, so go test ./..., go vet ./... and the skip report never see it,
// and no product code can import it. Run it in the dev container with
//
//	go test -count=1 -tags vaultwarden -v ./internal/e2e/
//
// A run fetches the pinned Vaultwarden image from Docker Hub (design D2),
// starts one server per scenario on loopback (D3), registers an account
// over the HTTP API (D4), seeds it through the official bw (D5) and runs
// bw-takeout backup under a pseudo-terminal (D6). It contacts Docker Hub
// and, once for each fresh bw profile, https://api.bitwarden.com.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// takeout is the bw-takeout command built once for all tests.
var takeout string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bw-takeout-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	takeout = filepath.Join(dir, "bw-takeout")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", takeout, "../../cmd/bw-takeout")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	restore, err := teeStdout()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	if !checkLog(restore()) && code == 0 {
		code = 1
	}
	os.RemoveAll(dir)
	os.Exit(code)
}

// repoRoot is the root of the repository, two levels above the package
// directory that go test runs in.
func repoRoot(t testing.TB) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repository root not found: %v", err)
	}
	return root
}
