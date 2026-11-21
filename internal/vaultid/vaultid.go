// Package vaultid holds the rules for the vault IDs that leave the CLI:
// item IDs, attachment IDs and organization IDs. The archive uses them for
// entry names and the bw client for arguments, so an ID that passes here can
// be neither a path nor an option. The package imports only the standard
// library. See the design of the change archive-generation, decision D3, of
// the change bw-process-adapter, decision D5, and of the change
// organization-export, decision D1.
package vaultid

// ValidItemID reports whether id has the UUID form of 8-4-4-4-12
// hexadecimal digits.
func ValidItemID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHex(c) {
				return false
			}
		}
	}
	return true
}

// ValidOrganizationID reports whether id has the UUID form of 8-4-4-4-12
// hexadecimal digits, the rule of item IDs.
func ValidOrganizationID(id string) bool {
	return ValidItemID(id)
}

// ValidAttachmentID reports whether id consists of 1 to 64 ASCII letters
// and digits.
func ValidAttachmentID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
