//go:build vaultwarden && linux

package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

// twoStepProviderAuthenticator is the provider number of the authenticator
// method in Vaultwarden.
const twoStepProviderAuthenticator = 0

// totpStep returns the step of a point in time.
func totpStep(now time.Time) uint64 {
	return uint64(now.Unix()) / totpStepSeconds
}

// clock returns the current time of the account.
func (a *account) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// nextCode returns the code that the account types at the code prompt
// (design D3): the code of the step max(current step, last used step + 1),
// after which that step is the last used step. A code is accepted by the
// server only for a step later than the last used one, and the server
// allows one step of drift, so the code of the next step is typed when the
// prompt appears in the step of the activation. The test never waits for a
// step boundary.
func (a *account) nextCode() (string, error) {
	if a.totpSecret == nil {
		return "", errors.New("two-step login is not enabled")
	}
	step := max(totpStep(a.clock()), a.lastStep+1)
	a.lastStep = step
	return totpCode(a.totpSecret, step), nil
}

// enableTwoStep enables the authenticator method of the account in two
// calls (design D2): get-authenticator returns a key, and authenticator
// activates the method with a code for the current step. The account then
// keeps the secret and that step as its last used step. The secret is
// recorded for the log check (design D6).
func (s *apiSession) enableTwoStep(a *account) error {
	body := map[string]string{"masterPasswordHash": a.keys.PasswordHash}
	var got struct {
		Enabled bool   `json:"enabled"`
		Key     string `json:"key"`
	}
	if err := s.do(http.MethodPost, "/api/two-factor/get-authenticator", body, &got); err != nil {
		return err
	}
	if got.Enabled {
		return errors.New("get-authenticator: two-step login is already enabled")
	}
	secret, err := decodeTOTPKey(got.Key)
	if err != nil {
		return fmt.Errorf("get-authenticator: %w", err)
	}
	keepOutOfLog("totp secret", got.Key)
	keepOutOfLog("totp secret", strings.ToLower(got.Key))

	step := totpStep(a.clock())
	err = s.do(http.MethodPost, "/api/two-factor/authenticator", map[string]string{
		"key":                got.Key,
		"token":              totpCode(secret, step),
		"masterPasswordHash": a.keys.PasswordHash,
	}, nil)
	if err != nil {
		return fmt.Errorf("authenticator: %w", err)
	}
	a.totpSecret = secret
	a.lastStep = step
	return nil
}

// passwordGrantWithoutCode sends the password grant with no two-step
// fields and returns the error of a refused grant: its description and the
// provider numbers the server lists.
func (a *account) passwordGrantWithoutCode(client *http.Client, base string) (description string, providers []int, err error) {
	form, err := a.grantForm()
	if err != nil {
		return "", nil, err
	}
	resp, err := client.PostForm(base+"/identity/connect/token", form)
	if err != nil {
		return "", nil, fmt.Errorf("connect/token: %w", err)
	}
	out, err := readBody(resp)
	if err != nil {
		return "", nil, fmt.Errorf("connect/token: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return "", nil, errors.New("connect/token: the grant without a code was accepted")
	}
	var e struct {
		Description string          `json:"error_description"`
		Providers   json.RawMessage `json:"TwoFactorProviders"`
	}
	if err := json.Unmarshal(out, &e); err != nil {
		return "", nil, fmt.Errorf("connect/token: status %d, the body is not JSON", resp.StatusCode)
	}
	if len(e.Providers) > 0 {
		// The server lists the numbers as strings or as numbers.
		var raw []any
		if err := json.Unmarshal(e.Providers, &raw); err != nil {
			return "", nil, errors.New("connect/token: TwoFactorProviders is not a list")
		}
		for _, v := range raw {
			var n int
			switch v := v.(type) {
			case float64:
				n = int(v)
			case string:
				if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
					return "", nil, errors.New("connect/token: a provider is not a number")
				}
			default:
				return "", nil, errors.New("connect/token: a provider is not a number")
			}
			providers = append(providers, n)
		}
		sort.Ints(providers)
	}
	return e.Description, providers, nil
}

// enableTwoStepChecked logs in over the API, enables two-step login and
// checks that a grant without a code is refused with the authenticator as
// the only provider (design D2). A single provider means that bw shows no
// method chooser before the code prompt.
func enableTwoStepChecked(t *testing.T, s *vaultServer, a *account) {
	t.Helper()
	api, err := a.login(s.Client, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.enableTwoStep(a); err != nil {
		t.Fatal(err)
	}
	desc, providers, err := a.passwordGrantWithoutCode(s.Client, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if desc != "Two factor required." {
		t.Fatalf("error_description %q, want %q", desc, "Two factor required.")
	}
	if len(providers) != 1 || providers[0] != twoStepProviderAuthenticator {
		t.Fatalf("TwoFactorProviders %v, want exactly [%d]", providers, twoStepProviderAuthenticator)
	}
}

// codePrompt is the prompt of bw for the two-step code.
const codePrompt = "Two-step login code"

// twoStepAnswers are the answers of an account with two-step login. The
// code answer logs the step it typed relative to the step of the
// activation, never the code.
func twoStepAnswers(t *testing.T, a *account) map[string]answerFunc {
	activation := a.lastStep
	return map[string]answerFunc{
		"Email address":   fixed(a.Email),
		"Master password": fixed(a.Password),
		codePrompt: func() (string, error) {
			code, err := a.nextCode()
			t.Logf("code typed for the step %+d relative to the activation, current step %+d", int64(a.lastStep)-int64(activation), int64(totpStep(a.clock()))-int64(activation))
			return code, err
		},
	}
}

// TestTwoStepEnrollment enables two-step login on a started server and
// checks that a password grant without a code is refused with the
// authenticator as the only provider (design D2).
func TestTwoStepEnrollment(t *testing.T) {
	s := startServer(t)
	a := newAccount(t)
	if err := a.register(s.Client, s.URL); err != nil {
		t.Fatal(err)
	}
	api, err := a.login(s.Client, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.enableTwoStep(a); err != nil {
		t.Fatal(err)
	}
	if len(a.totpSecret) != totpKeyBytes || a.lastStep == 0 {
		t.Fatal("the account does not keep the secret and the step")
	}
	desc, providers, err := a.passwordGrantWithoutCode(s.Client, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if desc != "Two factor required." {
		t.Errorf("error_description %q, want %q", desc, "Two factor required.")
	}
	if len(providers) != 1 || providers[0] != twoStepProviderAuthenticator {
		t.Errorf("TwoFactorProviders %v, want exactly [%d]", providers, twoStepProviderAuthenticator)
	}
}
