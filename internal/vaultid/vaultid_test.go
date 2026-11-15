package vaultid_test

import (
	"strings"
	"testing"

	"bw-takeout/internal/vaultid"
)

const (
	item       = "11111111-2222-4333-8444-555555555555"
	attachment = "f00dfeed0123456789abcdef"
)

func TestValidIDs(t *testing.T) {
	if !vaultid.ValidItemID(item) || !vaultid.ValidItemID(strings.ToUpper(item)) {
		t.Fatal("valid item ID refused")
	}
	if !vaultid.ValidAttachmentID(attachment) || !vaultid.ValidAttachmentID(strings.Repeat("Z", 64)) {
		t.Fatal("valid attachment ID refused")
	}
}

func TestInvalidIDs(t *testing.T) {
	bad := map[string]string{
		"empty":            "",
		"leading dash":     "-" + item[1:],
		"path separator":   "11111111/2222-4333-8444-555555555555",
		"backslash":        `11111111\2222-4333-8444-555555555555`,
		"parent reference": "..",
		"line break":       "11111111-2222-4333-8444-55555555555\n",
		"space":            " 1111111-2222-4333-8444-555555555555",
		"short":            item[:35],
		"long":             item + "0",
	}
	for name, id := range bad {
		if vaultid.ValidItemID(id) {
			t.Errorf("item ID %s accepted", name)
		}
		if vaultid.ValidAttachmentID(id) {
			t.Errorf("attachment ID %s accepted", name)
		}
	}
	if vaultid.ValidAttachmentID(strings.Repeat("a", 65)) {
		t.Error("attachment ID of 65 characters accepted")
	}
	if vaultid.ValidItemID("11111111-2222-4333-8444-55555555555g") {
		t.Error("item ID with a non-hexadecimal digit accepted")
	}
}
