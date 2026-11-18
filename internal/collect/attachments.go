package collect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/capture"
	"bw-takeout/internal/vaultid"
)

// MaxAttempts is the number of attempts of one download that failed in a
// way a retry can survive (design D8).
const MaxAttempts = 3

// retryWaits are the waits before the second and the third attempt.
var retryWaits = [MaxAttempts - 1]time.Duration{2 * time.Second, 5 * time.Second}

// Receiver takes one downloaded attachment. Content is valid only until the
// receiver returns; the collection then overwrites it with zero bytes.
// (*archive.Writer).AddAttachment is a receiver.
type Receiver func(archive.Attachment) error

// Attachments downloads the planned attachments of the snapshot in order,
// one at a time, and hands each successful download to receive before it
// requests the next one (designs D2, D7, D8 and D10). It returns exactly one
// download result per planned attachment.
//
// A failure of the receiver, a cancelled context and a failure of a
// download that concerns the run as a whole end the collection: no download
// results are returned. A failure of the receiver is returned as
// [ErrReceive] with the receiver's failure as its cause; any other failure is
// returned as it is.
func (c *Collector) Attachments(ctx context.Context, v Vault, s *Snapshot, receive Receiver) ([]capture.Download, error) {
	downloads := make([]capture.Download, 0, len(s.Plan))
	for _, p := range s.Plan {
		out, class, err := c.download(ctx, v, p)
		if err != nil {
			return nil, err
		}
		d := capture.Download{ItemID: p.ItemID, AttachmentID: p.AttachmentID}
		if class != "" {
			d.Error = class
			downloads = append(downloads, d)
			continue
		}
		sum := sha256.Sum256(out.Data)
		d.SHA256 = hex.EncodeToString(sum[:])
		d.Size = int64(len(out.Data))
		d.DecryptionErrors = out.DecryptionErrors
		err = receive(archive.Attachment{
			ItemID:       p.ItemID,
			AttachmentID: p.AttachmentID,
			FileName:     p.FileName,
			Content:      out.Data,
		})
		clear(out.Data)
		if err != nil {
			return nil, &receiveError{cause: err}
		}
		downloads = append(downloads, d)
	}
	return downloads, nil
}

// download requests one attachment with the retries of design D8. It
// returns the content on success, a download error class for a failed
// download, or an error that ends the collection.
func (c *Collector) download(ctx context.Context, v Vault, p PlannedAttachment) (bwcli.Output, capture.ErrorClass, error) {
	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return bwcli.Output{}, "", cancelled(p)
		}
		out, err := v.Attachment(ctx, p.ItemID, p.AttachmentID)
		if err == nil {
			return out, "", nil
		}
		clear(out.Data)
		var e *bwcli.Error
		if !errors.As(err, &e) {
			return bwcli.Output{}, "", vaultError(bwcli.OpAttachment, err)
		}
		retry, class, failed := classify(e.Class)
		switch {
		case !failed:
			return bwcli.Output{}, "", e
		case !retry || attempt == MaxAttempts:
			return bwcli.Output{}, class, nil
		}
		if err := c.wait(ctx, retryWaits[attempt-1]); err != nil {
			return bwcli.Output{}, "", cancelled(p)
		}
	}
}

// classify maps the class of a failed download to the table of design D7.
// failed is false for a failure that ends the collection. Any class the
// table does not name ends the collection, so that an unknown case is never
// treated more leniently than a known one.
func classify(class bwcli.Class) (retry bool, result capture.ErrorClass, failed bool) {
	switch class {
	case bwcli.ClassTimeout:
		return true, capture.ErrorProcess, true
	case bwcli.ClassExit:
		return true, capture.ErrorStatus, true
	case bwcli.ClassOutput:
		// On a download, output means only that the content passed the
		// output limit. A second attempt meets the same limit.
		return false, capture.ErrorOther, true
	case bwcli.ClassPremiumRequired:
		return false, capture.ErrorStatus, true
	case bwcli.ClassInvalidArgument:
		return false, capture.ErrorOther, true
	}
	return false, "", false
}

// cancelled is the failure of a download that the cancelled run did not
// start. Like a failure of bwcli, it names the attachment only when both IDs
// have the valid form.
func cancelled(p PlannedAttachment) error {
	e := &bwcli.Error{Op: bwcli.OpAttachment, Class: bwcli.ClassCancelled, ExitCode: bwcli.NoExitCode}
	if vaultid.ValidItemID(p.ItemID) && vaultid.ValidAttachmentID(p.AttachmentID) {
		e.ItemID, e.AttachmentID = p.ItemID, p.AttachmentID
	}
	return e
}
