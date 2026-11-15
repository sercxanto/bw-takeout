package bwcli

import "time"

// command is one non-interactive bw call (design D5).
type command struct {
	op      Op
	args    []string
	timeout time.Duration
	// session gives the process the session key, if the client holds one.
	session bool
	// itemID and attachmentID are set for a download and name it in a
	// failure. They are validated before the command is built.
	itemID, attachmentID string
}

func (c *Client) cmdVersion() command {
	return command{op: OpVersion, args: []string{"--version"}, timeout: c.cfg.Timeouts.Short}
}

func (c *Client) cmdStatus() command {
	return command{op: OpStatus, args: []string{"status"}, timeout: c.cfg.Timeouts.Short, session: true}
}

func (c *Client) cmdServer(url string) command {
	return command{op: OpServer, args: []string{"config", "server", url}, timeout: c.cfg.Timeouts.Short}
}

func (c *Client) cmdSync() command {
	return command{op: OpSync, args: []string{"sync", "--force", "--raw"}, timeout: c.cfg.Timeouts.Long, session: true}
}

func (c *Client) cmdExportJSON() command {
	return command{op: OpExportJSON, args: []string{"export", "--format", "json", "--raw"}, timeout: c.cfg.Timeouts.Long, session: true}
}

func (c *Client) cmdExportEncrypted() command {
	return command{op: OpExportEncrypted, args: []string{"export", "--format", "encrypted_json", "--raw"}, timeout: c.cfg.Timeouts.Long, session: true}
}

func (c *Client) cmdListItems() command {
	return command{op: OpListItems, args: []string{"list", "items", "--raw"}, timeout: c.cfg.Timeouts.Long, session: true}
}

func (c *Client) cmdListArchived() command {
	return command{op: OpListArchived, args: []string{"list", "items", "--archived", "--raw"}, timeout: c.cfg.Timeouts.Long, session: true}
}

func (c *Client) cmdAttachment(itemID, attachmentID string) command {
	return command{
		op:      OpAttachment,
		args:    []string{"get", "attachment", attachmentID, "--itemid", itemID, "--raw"},
		timeout: c.cfg.Timeouts.Attachment, session: true,
		itemID: itemID, attachmentID: attachmentID,
	}
}

func (c *Client) cmdLogout() command {
	return command{op: OpLogout, args: []string{"logout"}, timeout: c.cfg.Timeouts.Short, session: true}
}
