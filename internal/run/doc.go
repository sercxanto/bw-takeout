// Package run makes one interactive backup of one account: it checks the
// inputs, creates a profile, logs in at the terminal, collects and verifies
// a capture, publishes a generation by its verdict and tears down on every
// path. It returns an Outcome, from which package report renders the final
// report and the exit status. See the design of the change
// interactive-backup-run, decisions D3 to D7
// (openspec/changes/archive/2026-11-18-interactive-backup-run/design.md),
// and the specification backup-run.
//
// # Order
//
//  1. The checks before the first bw process (D3): crash dumps are limited
//     (design D2 of the change memory-hardening), the target is an existing
//     directory, the recipients file yields at least one recipient, bw
//     resolves to a regular file, a given CA file is a regular file, and
//     the partial generations of the target are listed.
//  2. Profile, leftovers, version, server and login (D4).
//  3. The snapshot, while the target is still untouched.
//  4. Inside the publishing step: the archive writer, the attachments, the
//     capture, the wipe of the four documents, the verification and the
//     manifest.
//  5. Teardown (D5): a logout after every login attempt, bounded by
//     Deps.LogoutLimit, then the removal of the profile.
//
// # Interrupts
//
// The signals on Deps.Signals are read by one goroutine (D6). The first
// one sets the interrupted flag and cancels the run context. A signal that
// arrives once teardown has started, or any later signal, cancels the
// logout. A signal before the logout starts lets the logout end without
// starting bw. Publishing takes no context, so a commit that has begun
// finishes.
//
// # Output
//
// The package writes nothing itself. The lines that appear while the run
// goes on go to Deps.Progress. The login process of bw prompts on
// Deps.Stderr.
//
// # Evidence level
//
// The tests run the real bwcli.Client against the fake bw of package
// bwclitest. Their evidence level is mock.
package run
