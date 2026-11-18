package bwcli

import (
	"path/filepath"
	"testing"
	"time"
)

// The default limit of one attachment download is 5 minutes, and a limit
// the caller sets is kept (design D12 of capture-collection).
func TestAttachmentTimeoutDefault(t *testing.T) {
	dir := t.TempDir()
	base := Config{
		Executable: filepath.Join(dir, "bw"),
		DataDir:    filepath.Join(dir, "data"),
		WorkDir:    filepath.Join(dir, "work"),
	}
	c, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.cfg.Timeouts.Attachment; got != 5*time.Minute {
		t.Errorf("default attachment limit = %v, want 5m", got)
	}

	set := base
	set.Timeouts.Attachment = 42 * time.Second
	c, err = New(set)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.cfg.Timeouts.Attachment; got != 42*time.Second {
		t.Errorf("attachment limit = %v, want the 42s the caller set", got)
	}
}
