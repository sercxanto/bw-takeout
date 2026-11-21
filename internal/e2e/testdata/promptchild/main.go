//go:build vaultwarden && linux

// Command promptchild prompts on standard error and reads standard input,
// like bw login. Its argument picks the case: known (two prompts), code
// (a third prompt, Two-step login code), unknown and hang. It prints to standard output
// whether the answers matched, so the test sees what arrived without the
// answers themselves. The pseudo-terminal tests of internal/e2e build it
// with the tag vaultwarden.
package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

func ask(r *bufio.Reader, prompt string) string {
	fmt.Fprint(os.Stderr, "\x1b[?25l? "+prompt+": \x1b[?25h")
	line, _ := r.ReadString('\n')
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	fmt.Fprint(os.Stderr, "\r\x1b[2K? "+prompt+": done\n")
	return line
}

func main() {
	if !term.IsTerminal(0) || !term.IsTerminal(2) {
		fmt.Println("no terminal")
		os.Exit(3)
	}
	r := bufio.NewReader(os.Stdin)
	email := ask(r, "Email address")
	switch os.Args[1] {
	case "unknown":
		// A prompt name that the suite never answers.
		ask(r, "Security question")
	case "hang":
		time.Sleep(time.Hour)
	}
	password := ask(r, "Master password")
	if os.Args[1] == "code" {
		code := ask(r, "Two-step login code")
		fmt.Println(email == os.Getenv("WANT_EMAIL"), password == os.Getenv("WANT_PASSWORD"), code == os.Getenv("WANT_CODE"))
		return
	}
	fmt.Println(email == os.Getenv("WANT_EMAIL"), password == os.Getenv("WANT_PASSWORD"))
}
