package main

import (
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"bw-takeout/internal/archive"
)

var flagPattern = regexp.MustCompile(`--[a-z][a-z-]*`)

// flagsIn returns the sorted set of long flags in text.
func flagsIn(text string) []string {
	found := flagPattern.FindAllString(text, -1)
	slices.Sort(found)
	return slices.Compact(found)
}

// section returns the text of a level-two section of a Markdown page.
func section(t *testing.T, page, heading string) string {
	t.Helper()
	_, rest, ok := strings.Cut(page, "\n## "+heading+"\n")
	if !ok {
		t.Fatalf("no section %q", heading)
	}
	if i := strings.Index(rest, "\n## "); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// codeBlocks returns the fenced code blocks of text with the given
// language.
func codeBlocks(text, lang string) []string {
	var out []string
	for _, part := range strings.Split(text, "```"+lang+"\n")[1:] {
		block, _, _ := strings.Cut(part, "```")
		out = append(out, block)
	}
	return out
}

// usagePage reads the usage page with line feeds only. Git on the Windows
// runners checks text files out with CRLF line endings.
func usagePage(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../docs/user/usage.md")
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// The command line and the flag table of the usage page match the usage
// text of the command.
func TestUsagePageMatchesBackupFlags(t *testing.T) {
	page := usagePage(t)
	sec := section(t, page, "Making a Backup")
	blocks := codeBlocks(sec, "sh")
	if len(blocks) == 0 {
		t.Fatal("no command line on the page")
	}
	synopsis, _, _ := strings.Cut(backupUsage, "\n\n")
	want := flagsIn(synopsis)
	if len(want) != 6 {
		t.Fatalf("usage text names %v", want)
	}
	if got := flagsIn(blocks[0]); !slices.Equal(got, want) {
		t.Errorf("command line of the page names %v, usage text %v", got, want)
	}
	var table []string
	for _, line := range strings.Split(sec, "\n") {
		if strings.HasPrefix(line, "| `--") {
			cell, _, _ := strings.Cut(strings.TrimPrefix(line, "| "), " |")
			table = append(table, flagsIn(cell)...)
		}
	}
	slices.Sort(table)
	if !slices.Equal(table, want) {
		t.Errorf("flag table of the page names %v, usage text %v", table, want)
	}
	_, flagList, _ := strings.Cut(backupUsage, "Flags:\n")
	flagList, _, _ = strings.Cut(flagList, "\n\n")
	if got := flagsIn(flagList); !slices.Equal(got, want) {
		t.Errorf("flag list of the usage text names %v, synopsis %v", got, want)
	}
	// Every flag the usage text names is one the command accepts.
	args := []string{"--server", "https://vault.example.org", "--target", "t", "--recipients-file", "r", "--bw", "b", "--ca", "c",
		"--organization", "0c000000-0000-4000-8000-00000000000a"}
	if backupFlags(args) == nil || !slices.Equal(flagsIn(strings.Join(args, " ")), want) {
		t.Errorf("the command does not accept %v", want)
	}
}

// Review fix 9.6 (S4): the example of the usage page holds placeholders
// that do not parse, so that a copied example is refused instead of
// encrypting to keys of unknown holders.
func TestUsagePageRecipientsExample(t *testing.T) {
	page := usagePage(t)
	blocks := codeBlocks(section(t, page, "Making a Backup"), "text")
	if len(blocks) != 1 {
		t.Fatalf("%d text blocks", len(blocks))
	}
	var keys int
	for _, line := range strings.Split(blocks[0], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keys++
		if _, err := archive.ParseRecipients([]string{line}); err == nil {
			t.Errorf("example line %q is a valid recipient", line)
		}
	}
	if keys != 2 {
		t.Errorf("example holds %d recipient lines, want 2", keys)
	}
	if _, err := archive.ParseRecipientFile(strings.NewReader(blocks[0])); !errors.Is(err, archive.ErrRecipientFile) {
		t.Errorf("example file accepted: %v", err)
	}
}

// The security page states that the verify command does not limit crash
// dumps, which only a backup run does (review of 19 November 2026 of the
// change memory-hardening, finding S1).
func TestSecurityPageNamesVerifyCrashDumps(t *testing.T) {
	data, err := os.ReadFile("../../docs/user/security.md")
	if err != nil {
		t.Fatal(err)
	}
	sec := section(t, strings.ReplaceAll(string(data), "\r\n", "\n"), "Swap, Hibernation and Core Dumps")
	if !strings.Contains(sec, "`bw-takeout verify` limits nothing") {
		t.Error("the section does not say that verify limits no crash dumps")
	}
}
