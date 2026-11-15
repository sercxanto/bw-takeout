package bwcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"bw-takeout/internal/vaultid"
)

// maxVersionLength bounds the version line (design D5).
const maxVersionLength = 64

// Identity is what the client reports about bw (design D5, D16). Neither
// value is judged: any version runs, and no message is produced for it.
type Identity struct {
	// Version is the line bw --version printed, without the line ending.
	Version string
	// SHA256 is the hexadecimal SHA-256 of the file the executable path
	// resolves to after following symbolic links. In the npm layout this
	// is bw.js and does not cover the SDK module file beside it.
	SHA256 string
}

// Version queries the version of bw and hashes the executable. It then
// checks that bw uses the data directory: data.json must exist there after
// the query (design D5). Only the existence is checked; the file is never
// read. The login refuses to start until this check has passed.
func (c *Client) Version(ctx context.Context) (Identity, error) {
	sum, err := hashExecutable(c.cfg.Executable)
	if err != nil {
		return Identity{}, failure(OpVersion, ClassStart, NoExitCode)
	}
	out, err := c.run(ctx, c.cmdVersion())
	if err != nil {
		return Identity{}, err
	}
	version, ok := versionLine(out.stdout)
	if !ok {
		return Identity{}, failure(OpVersion, ClassOutput, 0)
	}
	fi, err := os.Lstat(filepath.Join(c.cfg.DataDir, dataFile))
	if err != nil || !fi.Mode().IsRegular() {
		return Identity{}, failure(OpVersion, ClassProfileNotUsed, 0)
	}
	c.profileUsed = true
	return Identity{Version: version, SHA256: sum}, nil
}

// dataFile is the file bw creates in a data directory it uses (notes 12).
const dataFile = "data.json"

func hashExecutable(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// versionLine accepts one line of 1 to 64 printable ASCII characters,
// optionally followed by one line ending.
func versionLine(b []byte) (string, bool) {
	s := string(b)
	if t, ok := strings.CutSuffix(s, "\r\n"); ok {
		s = t
	} else if t, ok := strings.CutSuffix(s, "\n"); ok {
		s = t
	}
	if len(s) == 0 || len(s) > maxVersionLength {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return "", false
		}
	}
	return s, true
}

// Status queries bw status and parses the document strictly (design D5).
// The state is read from the document and never from the exit code. A
// document that does not parse, an unknown state and a lastSync that is not
// a time are failures of the class output.
func (c *Client) Status(ctx context.Context) (Status, error) {
	return c.queryStatus(ctx)
}

// SetServer gives bw the server address. The address must be an HTTPS URL
// with a host, without user information and without a line break; any
// other address is refused before a process is started. bw refuses the
// command while an account is logged in (notes 11).
func (c *Client) SetServer(ctx context.Context, address string) error {
	if !validServerAddress(address) {
		return failure(OpServer, ClassInvalidArgument, NoExitCode)
	}
	return c.runDiscard(ctx, c.cmdServer(address))
}

func validServerAddress(address string) bool {
	if strings.ContainsAny(address, "\r\n") {
		return false
	}
	u, err := url.Parse(address)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.Opaque == "" && u.User == nil && u.Hostname() != ""
}

// Sync runs a forced sync. bw sync --force propagates a failure of the
// sync, unlike the sync inside the login (notes 7).
func (c *Client) Sync(ctx context.Context) error {
	return c.runDiscard(ctx, c.cmdSync())
}

// ExportJSON returns the plaintext JSON export as bw wrote it.
func (c *Client) ExportJSON(ctx context.Context) (Output, error) {
	return c.runDocument(ctx, c.cmdExportJSON())
}

// ExportEncryptedJSON returns the account-encrypted JSON export as bw wrote
// it.
func (c *Client) ExportEncryptedJSON(ctx context.Context) (Output, error) {
	return c.runDocument(ctx, c.cmdExportEncrypted())
}

// ListItems returns the item list without archived items as bw wrote it.
func (c *Client) ListItems(ctx context.Context) (Output, error) {
	return c.runDocument(ctx, c.cmdListItems())
}

// ListArchivedItems returns the list of archived items as bw wrote it.
func (c *Client) ListArchivedItems(ctx context.Context) (Output, error) {
	return c.runDocument(ctx, c.cmdListArchived())
}

// Attachment returns the content of one attachment, requested by item ID
// and attachment ID. Both IDs must have the form the archive accepts;
// another form is refused before a process is started. The content can be
// empty.
func (c *Client) Attachment(ctx context.Context, itemID, attachmentID string) (Output, error) {
	if !vaultid.ValidItemID(itemID) || !vaultid.ValidAttachmentID(attachmentID) {
		return Output{}, failure(OpAttachment, ClassInvalidArgument, NoExitCode)
	}
	out, err := c.run(ctx, c.cmdAttachment(itemID, attachmentID))
	if err != nil {
		return Output{}, err
	}
	return Output{Data: out.stdout, DecryptionErrors: out.decryptionErrors}, nil
}

// Logout runs bw logout and overwrites the session key whatever the
// outcome. It runs also without a session key, because a login whose key
// was refused can still have logged bw in.
func (c *Client) Logout(ctx context.Context) error {
	defer c.forgetSession()
	return c.runDiscard(ctx, c.cmdLogout())
}

func (c *Client) forgetSession() {
	clear(c.session)
	c.session = nil
}

// runDocument runs a command that returns a document. Output that is empty
// or only white space is not a document and fails with the class output.
// The document is otherwise returned unparsed (design D5).
func (c *Client) runDocument(ctx context.Context, cmd command) (Output, error) {
	out, err := c.run(ctx, cmd)
	if err != nil {
		return Output{}, err
	}
	if len(bytes.TrimSpace(out.stdout)) == 0 {
		return Output{}, failure(cmd.op, ClassOutput, 0)
	}
	return Output{Data: out.stdout, DecryptionErrors: out.decryptionErrors}, nil
}

// runDiscard runs a command whose result is nothing. Its standard output,
// a confirmation sentence of bw or nothing, is not read and is wiped.
func (c *Client) runDiscard(ctx context.Context, cmd command) error {
	out, err := c.run(ctx, cmd)
	clear(out.stdout)
	return err
}
