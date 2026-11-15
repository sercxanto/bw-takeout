package bwcli

import (
	"errors"
	"strconv"
)

// Op names an operation of the client.
type Op string

// The operations of design D5.
const (
	OpVersion         Op = "version"
	OpStatus          Op = "status"
	OpServer          Op = "server"
	OpLogin           Op = "login"
	OpSync            Op = "sync"
	OpExportJSON      Op = "export_json"
	OpExportEncrypted Op = "export_encrypted_json"
	OpListItems       Op = "list_items"
	OpListArchived    Op = "list_archived_items"
	OpAttachment      Op = "attachment"
	OpLogout          Op = "logout"
)

// Class is the kind of a failure (design D7).
type Class string

// The failure classes of design D7.
const (
	// ClassInvalidArgument: an ID or a server address was refused before
	// any start.
	ClassInvalidArgument Class = "invalid_argument"
	// ClassNoTerminal: a login without a terminal.
	ClassNoTerminal Class = "no_terminal"
	// ClassStart: the process could not be started.
	ClassStart Class = "start"
	// ClassTimeout: the process was stopped at its time limit.
	ClassTimeout Class = "timeout"
	// ClassCancelled: the process was stopped because the caller
	// cancelled.
	ClassCancelled Class = "cancelled"
	// ClassNotLoggedIn, ClassLocked and ClassPremiumRequired: a non-zero
	// exit code and a recognized sentence on standard error.
	ClassNotLoggedIn     Class = "not_logged_in"
	ClassLocked          Class = "locked"
	ClassPremiumRequired Class = "premium_required"
	// ClassExit: a non-zero exit code without a recognized sentence.
	ClassExit Class = "exit"
	// ClassOutput: exit code 0 with output of an unexpected form, or
	// output beyond the limit.
	ClassOutput Class = "output"
	// ClassStrayFile: a file appeared in the working directory.
	ClassStrayFile Class = "stray_file"
	// ClassProfileNotUsed: bw did not use the data directory.
	ClassProfileNotUsed Class = "profile_not_used"
)

// NoExitCode is the exit code of a failure without a process that ended
// with an exit code.
const NoExitCode = -1

// Error is a failed operation. It carries no text bw wrote, and its message
// is built from its fields only.
type Error struct {
	Op    Op
	Class Class
	// ExitCode is the exit code of bw, or NoExitCode.
	ExitCode int
	// ItemID and AttachmentID name the attachment of a failed download.
	// They are set only when both have the valid form.
	ItemID, AttachmentID string
}

func (e *Error) Error() string {
	s := "bw " + string(e.Op) + " failed: " + string(e.Class)
	if e.ExitCode != NoExitCode {
		s += ", exit code " + strconv.Itoa(e.ExitCode)
	}
	if e.ItemID != "" {
		s += ", item " + e.ItemID + ", attachment " + e.AttachmentID
	}
	return s
}

// ClassOf returns the class of a failure of this package, or "" for any
// other error.
func ClassOf(err error) Class {
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	return ""
}

func failure(op Op, class Class, exitCode int) *Error {
	return &Error{Op: op, Class: class, ExitCode: exitCode}
}
