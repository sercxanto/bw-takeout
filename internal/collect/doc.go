// Package collect collects one capture of a personal vault from a logged-in
// official CLI. It works in two phases (design D2 of the change
// capture-collection, openspec/changes/capture-collection/design.md):
//
//   - [Collector.Snapshot] forces one sync, checks that the snapshot is
//     fresh and collects the four documents, their decryption error counts,
//     the manifest source and the plan of attachment downloads.
//   - [Collector.Attachments] downloads the planned attachments one at a
//     time, hands each to a receiver and records one download result per
//     planned ID pair.
//
// The caller orders the phases around the target directory. It calls
// Snapshot before it touches the target, Attachments inside the write
// function of the publishing step, and [Snapshot.Wipe] once the documents
// were handed to the archive writer and parsed.
//
// The package works against the interface [Vault], which *bwcli.Client
// satisfies. It starts no process itself: only package bwcli knows how bw
// is started (design D1).
//
// # Rules
//
//   - Freshness (D3): the moment the sync starts, truncated to
//     milliseconds, must not lie after the last sync time of the status
//     that follows the sync, and the state must be unlocked.
//   - Documents (D4): plaintext export, encrypted export, default item list
//     and archived item list, in this order. A failure of any of them ends
//     the collection.
//   - Plan (D5, D6): the attachments of personal items, chosen by
//     capture.PersonalAttachments, with file names from the default list
//     first. When either list is rejected, the plan is empty.
//   - Downloads (D7, D8): a timeout or a non-zero exit code is tried up to
//     three times, with waits of 2 and 5 seconds. A premium failure, an
//     invalid ID and output that passes the output limit are failed
//     downloads without a retry. Every other failure ends the collection.
//   - Plaintext lifetime (D10): no step writes a file. See below for what
//     is cleared and what is not.
//
// # Plaintext lifetime
//
// Cleared (design D10 and D13):
//
//   - the four raw documents, by [Snapshot.Wipe], and the documents
//     collected before a failed document command, at once,
//   - the content of each attachment after the receiver returns, whether it
//     accepted the attachment or not, and the output of a failed attempt,
//     which bwcli clears,
//   - the arrays that the output buffer of bwcli leaves behind when it
//     grows.
//
// Not cleared, because Go strings cannot be overwritten and the garbage
// collector frees them at a time of its own:
//
//   - the decoded JSON trees of the parsers: the item lists that Snapshot
//     parses for the plan, the documents that capture.Parse reads, and the
//     plaintext export that archive.NewWriter parses to count items,
//   - the file names in [Snapshot.Plan] and the values in [Snapshot.Source].
//
// They can appear in a core dump, a swap file or a memory snapshot of the
// process. Hardening against that belongs to the caller.
//
// # Failures
//
// Every failure this package returns is a *bwcli.Error, which names the
// operation and the class and holds no text bw wrote, or one of the errors
// of this package. A failure of another type from a Vault is replaced by
// [ErrUnexpected], so that its text cannot reach a report. A failure of the
// receiver is returned as [ErrReceive], whose text is fixed; the receiver's
// failure stays reachable through errors.Unwrap for a caller that needs it.
//
// # Evidence level
//
// That list, export and get start no sync of their own, so that one forced
// sync fixes the snapshot for every later command, comes from reading the
// code of bw 2026.10.1 (docs/research/bw-cli-code-notes-2026-11-17.md). The
// tests of this package use an in-memory vault and the fake bw of package
// bwclitest. Their evidence level is mock.
package collect
