package bwcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// State is the state that bw status reports.
type State string

// The states of the 2026.10.1 bundle. bw status reports no other value
// (bundle lines 39608 to 39614; unchanged in 2026.11.1, bundle lines 40312
// to 40318).
const (
	StateUnauthenticated State = "unauthenticated"
	StateLocked          State = "locked"
	StateUnlocked        State = "unlocked"
)

// Status is the parsed status document (design D5). A string field is empty
// and LastSync is nil when the document holds no value for it.
type Status struct {
	ServerURL string
	UserID    string
	UserEmail string
	State     State
	LastSync  *time.Time
}

var errStatusForm = errors.New("status document of an unexpected form")

// parseStatus reads the status document strictly: one JSON object, nothing
// after it, a known state, string or null for the other fields, and an RFC
// 3339 time or null for lastSync. Fields this project does not know are
// ignored, so that an added field does not fail the status (design D16).
func parseStatus(b []byte) (Status, error) {
	var doc struct {
		ServerURL *string          `json:"serverUrl"`
		UserID    *string          `json:"userId"`
		UserEmail *string          `json:"userEmail"`
		Status    *string          `json:"status"`
		LastSync  *json.RawMessage `json:"lastSync"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&doc); err != nil {
		return Status{}, errStatusForm
	}
	if _, err := dec.Token(); err != io.EOF {
		return Status{}, errStatusForm
	}
	if doc.Status == nil {
		return Status{}, errStatusForm
	}
	st := Status{State: State(*doc.Status)}
	switch st.State {
	case StateUnauthenticated, StateLocked, StateUnlocked:
	default:
		return Status{}, errStatusForm
	}
	for _, f := range []struct {
		dst *string
		src *string
	}{{&st.ServerURL, doc.ServerURL}, {&st.UserID, doc.UserID}, {&st.UserEmail, doc.UserEmail}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if doc.LastSync != nil && string(*doc.LastSync) != "null" {
		var s string
		if err := json.Unmarshal(*doc.LastSync, &s); err != nil {
			return Status{}, errStatusForm
		}
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return Status{}, errStatusForm
		}
		st.LastSync = &t
	}
	return st, nil
}

// queryStatus runs bw status and parses the document. The output is wiped
// after parsing. A document that does not parse is a failure of the class
// output.
func (c *Client) queryStatus(ctx context.Context) (Status, error) {
	out, _, err := c.start(ctx, c.cmdStatus())
	if err != nil {
		return Status{}, err
	}
	st, perr := parseStatus(out.stdout)
	clear(out.stdout)
	if perr != nil {
		return Status{}, failure(OpStatus, ClassOutput, 0)
	}
	return st, nil
}
