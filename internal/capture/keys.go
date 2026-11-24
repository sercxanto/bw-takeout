package capture

import "sort"

// OtherKey stands in a key path for an unknown key that is not an
// identifier (design D5 of the change bw-version-2026-11). The text of such
// a key is never copied out of the document.
const OtherKey = "<other>"

// maxIdentifierLength bounds the length of a key that may enter a path.
const maxIdentifierLength = 64

// shape lists the keys this project knows at one place of an export. Each
// known key maps to the place of its value: nil for a value that is not
// walked, or a nested place for an object or an array of objects.
type shape map[string]*nested

type nested struct {
	shape shape
	// array marks a value that is an array of objects of the shape.
	array bool
}

func known(keys ...string) shape {
	s := shape{}
	for _, k := range keys {
		s[k] = nil
	}
	return s
}

func (s shape) object(key string, of shape) shape {
	s[key] = &nested{shape: of}
	return s
}

func (s shape) arrayOf(key string, of shape) shape {
	s[key] = &nested{shape: of, array: true}
	return s
}

// The known keys of each place (design D4 of the change bw-version-2026-11).
// Each set is the union of the keys that the export model classes and the
// document types write in bw 2026.10.1 and 2026.11.1, which are equal
// (docs/research/bw-cli-code-notes-2026-11-24.md, "Export keys"). The item
// set is one set for both forms, so that a legacy item with key and a blob
// item with data pass alike.
var (
	passkeyKeys = known("credentialId", "keyType", "keyAlgorithm", "keyCurve", "keyValue",
		"rpId", "userHandle", "userName", "counter", "rpName", "userDisplayName",
		"discoverable", "creationDate")
	uriKeys   = known("uri", "uriChecksum", "match")
	loginKeys = known("username", "password", "totp").
			arrayOf("uris", uriKeys).
			arrayOf("fido2Credentials", passkeyKeys)
	fieldKeys   = known("name", "value", "type", "linkedId")
	historyKeys = known("password", "lastUsedDate")

	secureNoteKeys = known("type")
	cardKeys       = known("cardholderName", "brand", "number", "expMonth", "expYear", "code")
	identityKeys   = known("title", "firstName", "middleName", "lastName", "address1",
		"address2", "address3", "city", "state", "postalCode", "country", "company",
		"email", "phone", "ssn", "username", "passportNumber", "licenseNumber")
	sshKeyKeys      = known("privateKey", "publicKey", "keyFingerprint")
	bankAccountKeys = known("bankName", "nameOnAccount", "accountType", "accountNumber",
		"routingNumber", "branchNumber", "pin", "swiftCode", "iban", "bankContactPhone")
	driversLicenseKeys = known("firstName", "middleName", "lastName", "dateOfBirth",
		"licenseNumber", "issuingCountry", "issuingState", "issueDate", "expirationDate",
		"issuingAuthority", "licenseClass")
	passportKeys = known("surname", "givenName", "dateOfBirth", "sex", "birthPlace",
		"nationality", "issuingCountry", "passportNumber", "passportType",
		"nationalIdentificationNumber", "issuingAuthority", "issueDate", "expirationDate")

	itemKeys = known("id", "collectionIds", "type", "folderId", "organizationId", "name",
		"notes", "favorite", "reprompt", "revisionDate", "creationDate", "deletedDate",
		"archivedDate", "key", "data").
		arrayOf("fields", fieldKeys).
		arrayOf("passwordHistory", historyKeys).
		object("login", loginKeys).
		object("secureNote", secureNoteKeys).
		object("card", cardKeys).
		object("identity", identityKeys).
		object("sshKey", sshKeyKeys).
		object("bankAccount", bankAccountKeys).
		object("driversLicense", driversLicenseKeys).
		object("passport", passportKeys)

	folderKeys     = known("id", "name")
	collectionKeys = known("id", "organizationId", "name", "externalId")

	documentKeys = map[Document]shape{
		PlaintextExportDocument: known("encrypted").
			arrayOf("folders", folderKeys).arrayOf("items", itemKeys),
		EncryptedExportDocument: known("encrypted", "encKeyValidation_DO_NOT_EDIT").
			arrayOf("folders", folderKeys).arrayOf("items", itemKeys),
		OrganizationPlaintextExportDocument: known("encrypted").
			arrayOf("collections", collectionKeys).arrayOf("items", itemKeys),
		OrganizationEncryptedExportDocument: known("encrypted", "encKeyValidation_DO_NOT_EDIT").
			arrayOf("collections", collectionKeys).arrayOf("items", itemKeys),
	}
)

// unknownKeys returns the key paths of a document that its known set does
// not hold, sorted by path, with the number of top-level elements that
// carry each (design D4). The value of an unknown key is never walked, and
// neither is a known key whose value has an unexpected type.
func unknownKeys(doc Document, root map[string]any) []UnknownKey {
	s := documentKeys[doc]
	counts := map[string]int{}
	top := map[string]bool{}
	for k, value := range root {
		n, ok := s[k]
		switch {
		case !ok:
			top[pathKey(k)] = true
		case n == nil:
		case n.array:
			elements, _ := value.([]any)
			for _, e := range elements {
				obj, ok := e.(map[string]any)
				if !ok {
					continue
				}
				seen := map[string]bool{}
				walkKeys(k+"[].", obj, n.shape, seen)
				for p := range seen {
					counts[p]++
				}
			}
		default:
			if obj, ok := value.(map[string]any); ok {
				walkKeys(k+".", obj, n.shape, top)
			}
		}
	}
	for p := range top {
		counts[p] = 1
	}
	if len(counts) == 0 {
		return nil
	}
	out := make([]UnknownKey, 0, len(counts))
	for p, n := range counts {
		out = append(out, UnknownKey{Path: p, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// walkKeys adds to seen the path of every unknown key of obj and of the
// known places below it.
func walkKeys(prefix string, obj map[string]any, s shape, seen map[string]bool) {
	for k, value := range obj {
		n, ok := s[k]
		if !ok {
			seen[prefix+pathKey(k)] = true
			continue
		}
		if n == nil {
			continue
		}
		if !n.array {
			if child, ok := value.(map[string]any); ok {
				walkKeys(prefix+k+".", child, n.shape, seen)
			}
			continue
		}
		elements, _ := value.([]any)
		for _, e := range elements {
			if child, ok := e.(map[string]any); ok {
				walkKeys(prefix+k+"[].", child, n.shape, seen)
			}
		}
	}
}

// pathKey returns an unknown key for a path: the key itself when it is an
// identifier, an ASCII letter followed by at most 63 ASCII letters or
// digits, and OtherKey otherwise (design D5).
func pathKey(k string) string {
	if len(k) == 0 || len(k) > maxIdentifierLength {
		return OtherKey
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		case '0' <= c && c <= '9' && i > 0:
		default:
			return OtherKey
		}
	}
	return k
}

// unknownItemTypes counts per type the integer item types that
// typeObjectKeys does not hold, sorted by type.
func unknownItemTypes(types map[int]int) []UnknownItemType {
	if len(types) == 0 {
		return nil
	}
	out := make([]UnknownItemType, 0, len(types))
	for typ, n := range types {
		out = append(out, UnknownItemType{Type: typ, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}
