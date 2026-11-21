//go:build vaultwarden && linux

package e2e

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression tests of the review fixes of 21 November 2026.

// R1: the output of verify names the account like the backup report.
func TestRedactAddressInVerifyOutput(t *testing.T) {
	const email = "e2e-0011223344556677@example.invalid"
	out := "Generation:  /tmp/x\nAccount:     " + email + " (15659bfa-b954-498d-b0b7-bd5c9a657fef)\nItems:       3\n"
	got := redactAddress(out, email)
	if strings.Contains(got, email) || !strings.Contains(got, "Account:     <address> (") {
		t.Fatalf("redacted output:\n%s", got)
	}
}

// R2: a missing profile location fails, an empty one passes, one with an
// entry fails.
func TestProfileLocationMustExistAndBeEmpty(t *testing.T) {
	cache := t.TempDir()
	if err := profileLocationError(cache); err == nil {
		t.Fatal("a missing profile location passed")
	}
	loc := filepath.Join(cache, "bw-takeout", "profiles")
	if err := os.MkdirAll(loc, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := profileLocationError(cache); err != nil {
		t.Fatalf("an empty profile location failed: %v", err)
	}
	if err := os.Mkdir(filepath.Join(loc, "left"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := profileLocationError(cache); err == nil {
		t.Fatal("a profile location with an entry passed")
	}
}

// R3: outside testdata, every Go file of the package is a test file with
// the build constraint of design D1, so that the package stays invisible
// without the tag.
func TestPackageFilesAreTaggedTests(t *testing.T) {
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".go" {
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			t.Errorf("%s is not a test file", p)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		first, _ := bufio.NewReader(f).ReadString('\n')
		if strings.TrimSpace(first) != "//go:build vaultwarden && linux" {
			t.Errorf("%s does not start with the build constraint", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
