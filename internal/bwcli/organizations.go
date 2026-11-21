package bwcli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"bw-takeout/internal/vaultid"
)

// Organization is one entry of bw list organizations (design D9 of the
// change organization-export). Type and Status are the numbers bw printed;
// this package does not judge them.
type Organization struct {
	ID string
	// Name is the name bw printed, or empty. It is vault-adjacent data and
	// must not reach a report.
	Name   string
	Type   int
	Status int
}

// The role and membership numbers that bw list organizations reported
// against Vaultwarden 1.37.4 (docs/research/organization-export-spike-2026-11-21.md).
// A manager invited with type 3 was reported as type 4.
const (
	OrganizationOwner   = 0
	OrganizationAdmin   = 1
	OrganizationUser    = 2
	OrganizationManager = 3
	OrganizationCustom  = 4
	MembershipInvited   = 0
	MembershipAccepted  = 1
	MembershipConfirmed = 2
)

// ListOrganizations returns the organizations of the account. The output
// must be a JSON array whose every entry holds an ID of UUID form and a
// numeric type and status; any other output is a failure of the class
// output. The raw output is wiped after decoding.
func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	cmd := c.cmdListOrganizations()
	out, err := c.run(ctx, cmd)
	if err != nil {
		return nil, err
	}
	orgs, ok := parseOrganizationList(out.stdout)
	if !ok {
		return nil, cmd.failure(ClassOutput, 0)
	}
	return orgs, nil
}

// parseOrganizationList decodes the list strictly and wipes b whatever the
// outcome.
func parseOrganizationList(b []byte) ([]Organization, bool) {
	defer clear(b)
	var doc []struct {
		ID     *string `json:"id"`
		Name   *string `json:"name"`
		Type   *int    `json:"type"`
		Status *int    `json:"status"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&doc); err != nil || doc == nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	orgs := make([]Organization, 0, len(doc))
	for _, e := range doc {
		if e.ID == nil || !vaultid.ValidOrganizationID(*e.ID) || e.Type == nil || e.Status == nil {
			return nil, false
		}
		o := Organization{ID: *e.ID, Type: *e.Type, Status: *e.Status}
		if e.Name != nil {
			o.Name = *e.Name
		}
		orgs = append(orgs, o)
	}
	return orgs, true
}
