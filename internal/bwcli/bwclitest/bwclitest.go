// Package bwclitest builds the fake bw of package fakebw and describes its
// scenario and its records. Only tests and the development tools use it
// (design D12 of the change bw-process-adapter).
//
// The fake finds its scenario through BITWARDENCLI_APPDATA_DIR, so a test
// that sees the fake follow its scenario has also seen that variable arrive.
package bwclitest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// File names the fake uses in the data directory.
const (
	ScenarioFile = "fakebw-scenario.json"
	RecordFile   = "fakebw-records.jsonl"
	// DataFile is the file real bw creates in a fresh data directory.
	DataFile = "data.json"
	// LoggedOutFile marks a fake that Behavior.LogOut has logged out.
	LoggedOutFile = "fakebw-logged-out"
)

// ListOrganizations is the argument list of the organization list, as a
// key of Scenario.Commands.
const ListOrganizations = "list organizations --raw"

// OrganizationExport and OrganizationEncryptedExport return the argument
// lists of the two organization exports, as keys of Scenario.Commands.
func OrganizationExport(id string) string {
	return "export --organizationid " + id + " --format json --raw"
}

func OrganizationEncryptedExport(id string) string {
	return "export --organizationid " + id + " --format encrypted_json --raw"
}

// StatusUnauthenticated is the status document the fake prints once it is
// logged out.
const StatusUnauthenticated = `{"serverUrl":null,"lastSync":null,"status":"unauthenticated"}`

// NotLoggedInLine is what bw writes to standard error for a vault command
// after it logged out.
const NotLoggedInLine = "You are not logged in.\n"

// SleepArg is the argument with which the fake starts its own child for
// Behavior.SpawnChild. Such a child only sleeps and writes no record.
const SleepArg = "--fakebw-sleep"

// Scenario states how the fake answers each command.
type Scenario struct {
	// Commands maps a command to its behavior. The fake looks up the full
	// argument list joined by single spaces first, then the first argument,
	// then "*".
	Commands map[string]Behavior `json:"commands"`
}

// Behavior is the answer of the fake to one command.
type Behavior struct {
	// Exit is the exit code. Any non-empty BW_CLEANEXIT turns a non-zero
	// code into 0, as in bw.
	Exit int `json:"exit,omitempty"`
	// Stdout is written to standard output StdoutRepeat times, at least
	// once.
	Stdout       string `json:"stdout,omitempty"`
	StdoutRepeat int    `json:"stdoutRepeat,omitempty"`
	// Stderr is written to standard error StderrRepeat times, at least
	// once.
	Stderr       string `json:"stderr,omitempty"`
	StderrRepeat int    `json:"stderrRepeat,omitempty"`
	// DelayMillis delays the answer.
	DelayMillis int `json:"delayMillis,omitempty"`
	// Hang makes the fake sleep until it is killed.
	Hang bool `json:"hang,omitempty"`
	// SpawnChild starts a child of the fake that sleeps until it is
	// killed. Its process ID is recorded.
	SpawnChild bool `json:"spawnChild,omitempty"`
	// WriteFile names a file the fake writes into its working directory.
	WriteFile string `json:"writeFile,omitempty"`
	// SkipData keeps the fake from creating data.json.
	SkipData bool `json:"skipData,omitempty"`
	// Prompt is written to standard error before standard input is read.
	Prompt string `json:"prompt,omitempty"`
	// ReadLine makes the fake read one line from standard input and record
	// it.
	ReadLine bool `json:"readLine,omitempty"`
	// Interrupt makes the fake end by an interrupt signal to itself after
	// the line was read. On Windows it exits with the code of a console
	// interrupt instead.
	Interrupt bool `json:"interrupt,omitempty"`
	// LogOut logs the fake out after this answer: every later start other
	// than the version query and the login then answers as bw 2026.10.1
	// does without a session, status with StatusUnauthenticated and any
	// other command with NotLoggedInLine and exit code 1. It models the
	// 401 of Vaultwarden on an organization export without the role,
	// after which bw logged itself out (design D9 of the change
	// organization-export).
	LogOut bool `json:"logOut,omitempty"`
}

// Record is what the fake writes for each start.
type Record struct {
	Args []string `json:"args"`
	Env  []string `json:"env"`
	Dir  string   `json:"dir"`
	// StdinEmpty reports whether standard input was at its end. It is nil
	// when the fake did not look, which it does not for login and for
	// ReadLine.
	StdinEmpty *bool  `json:"stdinEmpty,omitempty"`
	Line       string `json:"line,omitempty"`
	PID        int    `json:"pid"`
	ChildPID   int    `json:"childPid,omitempty"`
	// CoreLimit is the soft and the hard core file size limit the fake
	// saw at its start. It is nil on Windows, which has no such limit
	// (design D6 of the change memory-hardening).
	CoreLimit *[2]uint64 `json:"coreLimit,omitempty"`
}

// Lookup returns the environment value of name in the record, and whether
// the record holds it.
func (r Record) Lookup(name string) (string, bool) {
	for _, e := range r.Env {
		k, v, ok := cutEnv(e)
		if ok && envNameEqual(k, name) {
			return v, true
		}
	}
	return "", false
}

// EnvNames returns the names of the recorded environment.
func (r Record) EnvNames() []string {
	var names []string
	for _, e := range r.Env {
		if k, _, ok := cutEnv(e); ok {
			names = append(names, k)
		}
	}
	return names
}

func cutEnv(e string) (string, string, bool) {
	// On Windows an entry can start with "=", such as "=C:=C:\x".
	for i := 1; i < len(e); i++ {
		if e[i] == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return "", "", false
}

func envNameEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// ExecutableName is the file name of the built fake.
func ExecutableName() string {
	if runtime.GOOS == "windows" {
		return "bw.exe"
	}
	return "bw"
}

// Build builds the fake into dir with the go command of the caller and
// returns the absolute path of the executable.
func Build(dir string) (string, error) {
	abs, err := filepath.Abs(filepath.Join(dir, ExecutableName()))
	if err != nil {
		return "", err
	}
	goCmd, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("go command not found: %w", err)
	}
	cmd := exec.Command(goCmd, "build", "-o", abs, "bw-takeout/internal/bwcli/fakebw")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building the fake bw: %w\n%s", err, out)
	}
	return abs, nil
}

// WriteScenario writes s into the data directory.
func WriteScenario(dataDir string, s Scenario) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, ScenarioFile), b, 0o600)
}

// ReadScenario reads the scenario from the data directory.
func ReadScenario(dataDir string) (Scenario, error) {
	var s Scenario
	b, err := os.ReadFile(filepath.Join(dataDir, ScenarioFile))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}

// Lookup returns the behavior for the argument list.
func (s Scenario) Lookup(args []string) (Behavior, bool) {
	if len(args) > 0 {
		if b, ok := s.Commands[strings.Join(args, " ")]; ok {
			return b, true
		}
		if b, ok := s.Commands[args[0]]; ok {
			return b, true
		}
	}
	b, ok := s.Commands["*"]
	return b, ok
}

// AppendRecord appends r to the record file of the data directory.
func AppendRecord(dataDir string, r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, RecordFile), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(b, '\n'))
	return errors.Join(werr, f.Close())
}

// ReadRecords returns the records of the data directory in the order of
// the starts. A missing record file means no start.
func ReadRecords(dataDir string) ([]Record, error) {
	f, err := os.Open(filepath.Join(dataDir, RecordFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, sc.Err()
}
