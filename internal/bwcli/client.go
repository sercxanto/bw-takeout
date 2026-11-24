// Package bwcli is the only code that starts the official bw program. It
// builds the environment of each process from an allowlist, keeps the
// session key in the environment of child processes only, holds standard
// output in memory, and reports failures by operation, class and exit code
// without copying any text bw wrote. See the design of the change
// bw-process-adapter, decisions D2 to D8, and the specification
// bw-invocation.
//
// Every command except the login runs with BW_NOINTERACTION=true, an empty
// standard input, a time limit and an empty working directory. The login
// runs only with a terminal on standard input and standard error and reads
// the session key from a pipe.
//
// # Evidence level
//
// The behavior of bw this package relies on is code-level evidence for the
// 2026.10.1 bundle, the live probe against Vaultwarden on Linux arm64, or an
// unauthenticated run in the dev container. See
// docs/research/bw-cli-code-notes-2026-11-15.md and
// docs/research/vaultwarden-live-probe-2026-11-14.md. The code of 2026.11.1
// was checked against each of these assumptions, and the Vaultwarden suite
// passed with it (docs/research/bw-cli-code-notes-2026-11-24.md).
package bwcli

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Defaults of design D4 and D8.
const (
	DefaultShortTimeout = 60 * time.Second
	DefaultLongTimeout  = 10 * time.Minute
	// DefaultAttachmentTimeout covers the whole transfer of one attachment,
	// because bw downloads and decrypts before it writes. Five minutes
	// carry 100 MB at about 3 Mbit/s and 500 MB at about 14 Mbit/s. With
	// the three attempts of the collection, a hanging download costs at
	// most 15 minutes (design D12 of the change capture-collection, which
	// replaced the 30 minutes of design D4).
	DefaultAttachmentTimeout = 5 * time.Minute
	DefaultOutputLimit       = 1 << 30
)

// loginOutputLimit bounds standard output of the login (design D6).
const loginOutputLimit = 4096

// waitDelay bounds how long reading the pipes continues after the process
// ended or was stopped, for the case that another process still holds them.
const waitDelay = 2 * time.Second

// Timeouts are the time limits per kind of command. A zero value takes the
// default.
type Timeouts struct {
	// Short applies to version, status, server configuration and logout.
	Short time.Duration
	// Long applies to sync, exports and item lists.
	Long time.Duration
	// Attachment applies to the download of one attachment.
	Attachment time.Duration
}

// Config is the configuration of one client. One client serves one profile.
type Config struct {
	// Executable is the absolute path of bw.
	Executable string
	// DataDir is the CLI data directory of the run. It is given to bw in
	// BITWARDENCLI_APPDATA_DIR.
	DataDir string
	// WorkDir is the empty working directory of every bw process.
	WorkDir string
	// CAFile is an optional absolute path of an additional certificate
	// authority file. It is given to bw in NODE_EXTRA_CA_CERTS.
	CAFile string
	// Timeouts are the time limits. The login has none.
	Timeouts Timeouts
	// OutputLimit bounds standard output of one command in bytes. Zero
	// takes DefaultOutputLimit.
	OutputLimit int64
	// Stdin and Stderr are given to the login process. Nil takes
	// os.Stdin and os.Stderr.
	Stdin, Stderr *os.File
	// Terminal answers whether the login may start and keeps the terminal
	// state. Nil takes NewTerminal(Stdin, Stderr).
	Terminal Terminal
}

// Client starts bw for one profile. It holds the session key between a
// successful login and the logout. A client is not safe for concurrent use.
type Client struct {
	cfg     Config
	session []byte
	// profileUsed records that Version found data.json in the data
	// directory. The login does not start without it.
	profileUsed bool
}

// New checks the configuration and returns a client. It starts no process.
func New(cfg Config) (*Client, error) {
	for _, p := range []struct{ name, path string }{
		{"executable", cfg.Executable},
		{"data directory", cfg.DataDir},
		{"working directory", cfg.WorkDir},
	} {
		if p.path == "" || !filepath.IsAbs(p.path) {
			return nil, errors.New("bwcli: the " + p.name + " must be an absolute path")
		}
	}
	if cfg.CAFile != "" && !filepath.IsAbs(cfg.CAFile) {
		return nil, errors.New("bwcli: the certificate authority file must be an absolute path")
	}
	if cfg.DataDir == cfg.WorkDir {
		return nil, errors.New("bwcli: the data directory and the working directory must differ")
	}
	if cfg.Timeouts.Short <= 0 {
		cfg.Timeouts.Short = DefaultShortTimeout
	}
	if cfg.Timeouts.Long <= 0 {
		cfg.Timeouts.Long = DefaultLongTimeout
	}
	if cfg.Timeouts.Attachment <= 0 {
		cfg.Timeouts.Attachment = DefaultAttachmentTimeout
	}
	if cfg.OutputLimit <= 0 {
		cfg.OutputLimit = DefaultOutputLimit
	}
	if cfg.Stdin == nil {
		cfg.Stdin = os.Stdin
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Terminal == nil {
		cfg.Terminal = NewTerminal(cfg.Stdin, cfg.Stderr)
	}
	return &Client{cfg: cfg}, nil
}

// LoggedIn reports whether the client holds a session key.
func (c *Client) LoggedIn() bool {
	return c.session != nil
}

// Output is the result of a command that returns content. Data is the
// standard output of bw, unparsed. DecryptionErrors is the number of
// decryption error lines bw wrote to standard error; it never turns a
// success into a failure (design D7).
type Output struct {
	Data             []byte
	DecryptionErrors int
}
