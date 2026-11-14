// Package verify turns one capture of a personal vault into a verdict of
// failed, unverified or verified, with a reason per affected ID. It is a
// pure function over the model of package capture and accesses no process,
// file or network. See the design of the change capture-verification.
package verify
