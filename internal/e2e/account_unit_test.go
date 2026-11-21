//go:build vaultwarden && linux

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRegistrationRequestBody checks the two request bodies on a test
// server: the fields of the flat form, and no field that holds the
// password.
func TestRegistrationRequestBody(t *testing.T) {
	const token = "verification-token"
	bodies := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		bodies[r.URL.Path] = v
		switch r.URL.Path {
		case "/identity/accounts/register/send-verification-email":
			w.Write([]byte(token))
		case "/identity/accounts/register/finish":
			w.Write([]byte(`{"object":"registerFinish"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	a := newAccount(t)
	if !strings.HasSuffix(a.Email, "@example.invalid") || a.Email != strings.ToLower(a.Email) {
		t.Fatalf("address form: %s", a.Email)
	}
	if err := a.register(srv.Client(), srv.URL); err != nil {
		t.Fatal(err)
	}

	send := bodies["/identity/accounts/register/send-verification-email"]
	if len(send) != 1 || send["email"] != a.Email {
		t.Fatalf("send-verification-email body has the fields %v", keysOf(send))
	}
	finish := bodies["/identity/accounts/register/finish"]
	want := []string{"email", "emailVerificationToken", "kdf", "kdfIterations", "masterPasswordHash", "key", "keys"}
	if got := keysOf(finish); strings.Join(got, ",") != strings.Join(sorted(want), ",") {
		t.Fatalf("register/finish body has the fields %v, want %v", got, sorted(want))
	}
	if finish["email"] != a.Email || finish["emailVerificationToken"] != token {
		t.Fatal("address or token differs")
	}
	if finish["kdf"] != float64(0) || finish["kdfIterations"] != float64(600000) {
		t.Fatalf("kdf %v, iterations %v", finish["kdf"], finish["kdfIterations"])
	}
	if finish["masterPasswordHash"] != a.keys.PasswordHash || !strings.HasPrefix(finish["key"].(string), "2.") {
		t.Fatal("hash or wrapped key differs")
	}
	keys, _ := finish["keys"].(map[string]any)
	if got := keysOf(keys); strings.Join(got, ",") != "encryptedPrivateKey,publicKey" {
		t.Fatalf("keys has the fields %v", got)
	}
	for path, body := range bodies {
		if containsString(body, a.Password) {
			t.Fatalf("%s: a field holds the password", path)
		}
	}
}

func TestRegistrationOnServer(t *testing.T) {
	s := startServer(t)
	a := newAccount(t)
	if err := a.register(s.Client, s.URL); err != nil {
		t.Fatal(err)
	}
	// A second registration of the same address is refused, which shows
	// that the first one created the account.
	again := &account{Email: a.Email, Password: a.Password}
	if err := again.register(s.Client, s.URL); err == nil {
		t.Fatal("the address was registered twice")
	}
}

// TestCipherPutReplacesKeyValue checks design D7 before any scenario
// builds on it: Vaultwarden accepts a PUT with a replaced keyValue, and a
// second GET returns the new value. The cipher is created over the API
// with EncStrings under the user key.
func TestCipherPutReplacesKeyValue(t *testing.T) {
	s := startServer(t)
	a := newAccount(t)
	if err := a.register(s.Client, s.URL); err != nil {
		t.Fatal(err)
	}
	api, err := a.login(s.Client, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	enc := func(v string) string {
		e, err := encString(a.keys.userKey, []byte(v))
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	var created map[string]any
	err = api.do(http.MethodPost, "/api/ciphers", map[string]any{
		"type":         1,
		"name":         enc("probe"),
		"encryptedFor": api.userID,
		"login": map[string]any{
			"username": enc("probe"),
			"fido2Credentials": []any{map[string]any{
				"credentialId": enc("00000000-0000-4000-8000-000000000000"),
				"keyType":      enc("public-key"),
				"keyAlgorithm": enc("ECDSA"),
				"keyCurve":     enc("P-256"),
				"keyValue":     enc("synthetic key value"),
				"rpId":         enc("passkey.example.invalid"),
				"counter":      enc("0"),
				"discoverable": enc("true"),
				"creationDate": "2026-11-21T00:00:00.000Z",
			}},
		},
	}, &created)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("no cipher ID")
	}
	if err := api.tamperPasskey(id); err != nil {
		t.Fatal(err)
	}
	t.Logf("PUT of cipher %s accepted, the second GET returned the new keyValue", id)
}

func keysOf(m map[string]any) []string {
	var k []string
	for name := range m {
		k = append(k, name)
	}
	return sorted(k)
}

func sorted(s []string) []string {
	out := append([]string{}, s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// containsString reports whether any string in a JSON value contains s.
func containsString(v any, s string) bool {
	switch x := v.(type) {
	case string:
		return strings.Contains(x, s)
	case map[string]any:
		for k, e := range x {
			if strings.Contains(k, s) || containsString(e, s) {
				return true
			}
		}
	case []any:
		for _, e := range x {
			if containsString(e, s) {
				return true
			}
		}
	}
	return false
}
