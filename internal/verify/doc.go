// Package verify turns one capture of a personal vault and of the chosen
// organizations into a verdict of failed, unverified or verified, with a
// reason per affected ID. It is a pure function over the model of package
// capture and accesses no process, file or network. See the design of the
// change capture-verification.
//
// The chosen organizations are those of capture.Capture.Organizations. Each
// one gets the checks of the personal exports, a membership check, a
// revision comparison between its two exports and with the item lists, and
// its attachments in the accounting. Its findings carry its ID and join the
// one verdict, and the result names it in its scope together with the limit
// that its exports are separate reads from the server. Items of other
// organizations are outside the scope and produce no finding (design D5 of
// the change organization-export).
package verify
