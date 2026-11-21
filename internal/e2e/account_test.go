//go:build vaultwarden && linux

package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// account is one synthetic account. The password exists in memory only
// (design D9).
type account struct {
	Email    string
	Password string
	keys     *accountKeys
}

// newAccount creates a random address under example.invalid and a random
// password. Vaultwarden stores addresses in lowercase, so the address is
// lowercase from the start.
func newAccount(t testing.TB) *account {
	t.Helper()
	id := make([]byte, 8)
	pw := make([]byte, 24)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(pw); err != nil {
		t.Fatal(err)
	}
	a := &account{
		Email:    "e2e-" + hex.EncodeToString(id) + "@example.invalid",
		Password: base64.RawURLEncoding.EncodeToString(pw),
	}
	keepOutOfLog("address", a.Email)
	keepOutOfLog("password", a.Password)
	return a
}

// registerFinishRequest is the flat request form that Vaultwarden 1.37.4
// accepts (RegisterDataOld in src/api/core/accounts.rs).
type registerFinishRequest struct {
	Email                  string       `json:"email"`
	EmailVerificationToken string       `json:"emailVerificationToken"`
	Kdf                    int          `json:"kdf"`
	KdfIterations          int          `json:"kdfIterations"`
	MasterPasswordHash     string       `json:"masterPasswordHash"`
	Key                    string       `json:"key"`
	Keys                   registerKeys `json:"keys"`
}

type registerKeys struct {
	EncryptedPrivateKey string `json:"encryptedPrivateKey"`
	PublicKey           string `json:"publicKey"`
}

// register creates the account on the server in two calls (design D4).
func (a *account) register(client *http.Client, base string) error {
	body, err := json.Marshal(map[string]string{"email": a.Email})
	if err != nil {
		return err
	}
	resp, err := client.Post(base+"/identity/accounts/register/send-verification-email", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("send-verification-email: %w", err)
	}
	token, err := readBody(resp)
	if err != nil {
		return fmt.Errorf("send-verification-email: %w", err)
	}
	if resp.StatusCode != http.StatusOK || len(token) == 0 {
		return fmt.Errorf("send-verification-email: status %d%s", resp.StatusCode, serverMessage(token))
	}

	keys, err := deriveAccountKeys(a.Email, a.Password, kdfIterations)
	if err != nil {
		return err
	}
	body, err = json.Marshal(registerFinishRequest{
		Email:                  a.Email,
		EmailVerificationToken: strings.TrimSpace(string(token)),
		Kdf:                    0, // PBKDF2-SHA256
		KdfIterations:          kdfIterations,
		MasterPasswordHash:     keys.PasswordHash,
		Key:                    keys.WrappedUserKey,
		Keys:                   registerKeys{EncryptedPrivateKey: keys.EncryptedPrivateKey, PublicKey: keys.PublicKey},
	})
	if err != nil {
		return err
	}
	resp, err = client.Post(base+"/identity/accounts/register/finish", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("register/finish: %w", err)
	}
	out, err := readBody(resp)
	if err != nil {
		return fmt.Errorf("register/finish: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("register/finish: status %d%s", resp.StatusCode, serverMessage(out))
	}
	var finish struct {
		Object string `json:"object"`
	}
	if err := json.Unmarshal(out, &finish); err != nil || finish.Object != "registerFinish" {
		return fmt.Errorf("register/finish: the response is not registerFinish")
	}
	a.keys = keys
	return nil
}

// apiSession is an access token of the account for the Vaultwarden API
// (design D7). The token is never logged.
type apiSession struct {
	client *http.Client
	base   string
	token  string
	userID string
}

// login gets an access token with the password grant, with device fields
// of its own, and reads the user ID from the profile.
func (a *account) login(client *http.Client, base string) (*apiSession, error) {
	if a.keys == nil {
		return nil, errors.New("the account is not registered")
	}
	device := make([]byte, 16)
	if _, err := rand.Read(device); err != nil {
		return nil, err
	}
	device[6] = device[6]&0x0f | 0x40
	device[8] = device[8]&0x3f | 0x80
	h := hex.EncodeToString(device)
	form := url.Values{
		"grant_type":       {"password"},
		"username":         {a.Email},
		"password":         {a.keys.PasswordHash},
		"scope":            {"api offline_access"},
		"client_id":        {"cli"},
		"deviceIdentifier": {h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]},
		"deviceName":       {"bw-takeout-e2e"},
		"deviceType":       {"25"},
	}
	resp, err := client.PostForm(base+"/identity/connect/token", form)
	if err != nil {
		return nil, fmt.Errorf("connect/token: %w", err)
	}
	out, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("connect/token: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("connect/token: status %d%s", resp.StatusCode, serverMessage(out))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(out, &tok); err != nil || tok.AccessToken == "" {
		return nil, errors.New("connect/token: no access token in the response")
	}
	s := &apiSession{client: client, base: base, token: tok.AccessToken}
	var profile struct {
		ID string `json:"id"`
	}
	if err := s.do(http.MethodGet, "/api/accounts/profile", nil, &profile); err != nil {
		return nil, err
	}
	if profile.ID == "" {
		return nil, errors.New("profile: no user ID")
	}
	s.userID = profile.ID
	return s, nil
}

// do sends one API request with the access token. in and out are JSON
// values; out may be nil.
func (s *apiSession) do(method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	data, err := readBody(resp)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: status %d%s", method, path, resp.StatusCode, serverMessage(data))
	}
	if out == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(out)
}

// getCipher reads one cipher as a generic JSON object, so that a PUT
// sends back every field it got.
func (s *apiSession) getCipher(id string) (map[string]any, error) {
	var c map[string]any
	if err := s.do(http.MethodGet, "/api/ciphers/"+url.PathEscape(id), nil, &c); err != nil {
		return nil, err
	}
	return c, nil
}

// putCipher writes a cipher back. Vaultwarden needs the user ID in
// encryptedFor, and takes the revision date it returned as the client's
// copy.
func (s *apiSession) putCipher(id string, c map[string]any) error {
	c["encryptedFor"] = s.userID
	if rd, ok := c["revisionDate"].(string); ok {
		c["lastKnownRevisionDate"] = rd
	}
	return s.do(http.MethodPut, "/api/ciphers/"+url.PathEscape(id), c, nil)
}

// passkeyKeyValue returns login.fido2Credentials[i].keyValue of a cipher.
func passkeyKeyValue(c map[string]any, i int) (string, error) {
	cred, err := passkeyCredential(c, i)
	if err != nil {
		return "", err
	}
	v, ok := cred["keyValue"].(string)
	if !ok {
		return "", errors.New("the passkey has no keyValue")
	}
	return v, nil
}

func passkeyCredential(c map[string]any, i int) (map[string]any, error) {
	login, ok := c["login"].(map[string]any)
	if !ok {
		return nil, errors.New("the cipher has no login")
	}
	creds, ok := login["fido2Credentials"].([]any)
	if !ok || i >= len(creds) {
		return nil, fmt.Errorf("the cipher has no passkey %d", i)
	}
	cred, ok := creds[i].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("passkey %d is not an object", i)
	}
	return cred, nil
}

// tamperPasskey replaces the encrypted keyValue of the first passkey with a
// well-formed EncString of type 2 under a random 64-byte key, and checks
// with a second GET that the server holds the new value (design D7).
func (s *apiSession) tamperPasskey(cipherID string) error {
	c, err := s.getCipher(cipherID)
	if err != nil {
		return err
	}
	old, err := passkeyKeyValue(c, 0)
	if err != nil {
		return err
	}
	foreign := make([]byte, 64)
	if _, err := rand.Read(foreign); err != nil {
		return err
	}
	// The length of a PKCS#8 P-256 key, so that the ciphertext has the
	// size of a real keyValue.
	plain := make([]byte, 138)
	if _, err := rand.Read(plain); err != nil {
		return err
	}
	replaced, err := encString(splitKey(foreign), plain)
	if err != nil {
		return err
	}
	if replaced == old {
		return errors.New("the new keyValue equals the old one")
	}
	cred, err := passkeyCredential(c, 0)
	if err != nil {
		return err
	}
	cred["keyValue"] = replaced
	if err := s.putCipher(cipherID, c); err != nil {
		return err
	}
	again, err := s.getCipher(cipherID)
	if err != nil {
		return err
	}
	got, err := passkeyKeyValue(again, 0)
	if err != nil {
		return err
	}
	if got != replaced {
		return errors.New("the server does not return the new keyValue")
	}
	return nil
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// serverMessage returns the message field of a Vaultwarden error body,
// which names the refused condition and holds no secret.
func serverMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) != nil || e.Message == "" {
		return ""
	}
	m := e.Message
	if len(m) > 200 {
		m = m[:200]
	}
	return ": " + strings.ToValidUTF8(m, "?")
}
