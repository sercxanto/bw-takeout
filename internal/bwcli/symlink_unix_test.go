//go:build unix

package bwcli

import (
	"os"
	"path/filepath"
	"testing"

	"bw-takeout/internal/bwcli/bwclitest"
)

// The npm layout reaches bw.js through a symbolic link. The hash covers the
// file the link resolves to.
func TestVersionThroughASymbolicLink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "bw")
	if err := os.Symlink(fakePath, link); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "bw-second")
	if err := os.Symlink(link, second); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, scenario("--version", bwclitest.Behavior{Stdout: "2026.10.1\n"}), func(c *Config) { c.Executable = second })
	id, err := f.client.Version(bg)
	if err != nil {
		t.Fatal(err)
	}
	if want := sha256File(t, fakePath); id.SHA256 != want {
		t.Errorf("hash %s, want the hash of the target %s", id.SHA256, want)
	}
}
