package collect

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bw-takeout/internal/bwcli"
)

// Vault is what the collection needs from a logged-in client. The methods
// have the signatures of *bwcli.Client.
type Vault interface {
	Sync(ctx context.Context) error
	Status(ctx context.Context) (bwcli.Status, error)
	ExportJSON(ctx context.Context) (bwcli.Output, error)
	ExportEncryptedJSON(ctx context.Context) (bwcli.Output, error)
	ListItems(ctx context.Context) (bwcli.Output, error)
	ListArchivedItems(ctx context.Context) (bwcli.Output, error)
	Attachment(ctx context.Context, itemID, attachmentID string) (bwcli.Output, error)
}

var _ Vault = (*bwcli.Client)(nil)

// Errors of the collection. They carry no value from the status.
var (
	// ErrNotFresh: the status after the sync holds no last sync time, or
	// one before the moment the sync was started.
	ErrNotFresh = errors.New("collect: snapshot is not fresh")
	// ErrNotUnlocked: the status after the sync does not report the state
	// unlocked.
	ErrNotUnlocked = errors.New("collect: vault is not unlocked after the sync")
	// ErrUnexpected replaces a failure of a Vault that is not a
	// *bwcli.Error. Its text is dropped, because it could quote bw.
	ErrUnexpected = errors.New("collect: unexpected failure of a vault call")
	// ErrReceive: the receiver refused an attachment. The receiver's
	// failure is the cause, reachable through errors.Unwrap, and its text
	// is not repeated (design D2).
	ErrReceive = errors.New("collect: receiver refused an attachment")
)

// receiveError is a failure of the receiver. Its text is that of
// ErrReceive, and errors.Is finds both ErrReceive and the cause.
type receiveError struct{ cause error }

func (e *receiveError) Error() string        { return ErrReceive.Error() }
func (e *receiveError) Unwrap() error        { return e.cause }
func (e *receiveError) Is(target error) bool { return target == ErrReceive }

// Collector collects captures. The zero value uses the system clock and
// waits with a timer.
type Collector struct {
	// Now returns the current time. Tests fix it.
	Now func() time.Time
	// Wait waits for d or until ctx is done, and returns ctx.Err() in the
	// second case. Tests record the durations and return at once.
	Wait func(ctx context.Context, d time.Duration) error
}

func (c *Collector) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Collector) wait(ctx context.Context, d time.Duration) error {
	if c.Wait != nil {
		return c.Wait(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// vaultError returns the *bwcli.Error inside err, or ErrUnexpected with
// the operation for a failure of another type.
func vaultError(op bwcli.Op, err error) error {
	var e *bwcli.Error
	if errors.As(err, &e) {
		return e
	}
	return fmt.Errorf("%w: %s", ErrUnexpected, op)
}
