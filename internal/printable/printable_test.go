package printable

import "testing"

func TestClean(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"plain", "user@example.org", "user@example.org"},
		{"control characters", "a\tb\nc\rd\x00e\x7f", "a?b?c?d?e?"},
		{"terminal escape sequence", "x\x1b]0;title\x07\x1b[2Jy", "x?]0;title??[2Jy"},
		{"printable non-ASCII text", "Grüße, 東京 – ok", "Grüße, 東京 – ok"},
		{"invisible format characters", "a​b‮c", "a?b?c"},
		// strings.Map turns invalid UTF-8 into U+FFFD, which is printable.
		{"invalid UTF-8", "a\xffb", "a\ufffdb"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Clean(c.in); got != c.want {
				t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
