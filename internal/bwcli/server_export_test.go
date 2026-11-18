package bwcli_test

import (
	"testing"

	"bw-takeout/internal/bwcli"
)

func TestValidServerAddressExported(t *testing.T) {
	for _, c := range []struct {
		address string
		want    bool
	}{
		{"https://vault.example.org", true},
		{"https://vault.example.org:8443/bitwarden", true},
		{"http://vault.example.org", false},
		{"https://", false},
		{"https:///vault", false},
		{"https://user@vault.example.org", false},
		{"https://user:synthetic@vault.example.org", false},
	} {
		if got := bwcli.ValidServerAddress(c.address); got != c.want {
			t.Errorf("ValidServerAddress(%q) = %v, want %v", c.address, got, c.want)
		}
	}
}
