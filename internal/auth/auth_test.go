package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/robertguss/go-islands-starter/internal/testdb"
)

func TestOIDCValidation(t *testing.T) {
	st, fixture := testdb.New(t)
	ctx := context.Background()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	type expected struct{ nonce, challenge string }
	attempts := map[string]expected{}
	var mu sync.Mutex
	var issuer string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad request", 400)
				return
			}
			mode := r.Form.Get("code")
			mu.Lock()
			a := attempts[mode]
			mu.Unlock()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(hash[:]) != a.challenge {
				http.Error(w, "bad verifier", 400)
				return
			}
			claims := map[string]any{"iss": issuer, "sub": "alex", "aud": "client", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": a.nonce, "email": "alex@example.test", "email_verified": true, "hd": "example.test"}
			signing := key
			switch mode {
			case "issuer":
				claims["iss"] = "https://wrong.test"
			case "audience":
				claims["aud"] = "wrong"
			case "expired":
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "nonce":
				claims["nonce"] = "wrong"
			case "signature":
				signing = wrongKey
			case "unadmitted":
				claims["email"] = "unadmitted@example.test"
			case "unverified":
				claims["email_verified"] = false
			case "domain":
				claims["hd"] = "elsewhere.test"
			case "missing-domain":
				delete(claims, "hd")
			case "subject":
				claims["sub"] = "different"
			case "empty-subject":
				claims["sub"] = ""
			}
			signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: signing}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
			if e != nil {
				t.Error(e)
				w.WriteHeader(500)
				return
			}
			data, e := json.Marshal(claims)
			if e != nil {
				t.Error(e)
				return
			}
			signed, e := signer.Sign(data)
			if e != nil {
				t.Error(e)
				return
			}
			token, e := signed.CompactSerialize()
			if e != nil {
				t.Error(e)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-only", "token_type": "Bearer", "id_token": token})
		default:
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	issuer = provider.URL
	oidc, err := newOIDC(ctx, Service{Queries: st.Auth, Secure: true}, "client", "secret", "https://app.example.test", []string{"example.test"}, issuer)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"valid", "issuer", "audience", "expired", "nonce", "signature", "unadmitted", "unverified", "domain", "missing-domain", "subject", "empty-subject", "disabled", "expired-attempt", "wrong-state", "missing-cookie"} {
		t.Run(mode, func(t *testing.T) {
			start := httptest.NewRecorder()
			oidc.Start(start, httptest.NewRequest("GET", "https://app.example.test/auth/login", nil))
			if start.Code != 302 {
				t.Fatalf("start: %d", start.Code)
			}
			location, err := url.Parse(start.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			state := location.Query().Get("state")
			if location.Query().Get("code_challenge_method") != "S256" {
				t.Fatal("PKCE missing")
			}
			mu.Lock()
			attempts[mode] = expected{nonce: location.Query().Get("nonce"), challenge: location.Query().Get("code_challenge")}
			mu.Unlock()
			if mode == "disabled" {
				if _, err = fixture.Exec(ctx, "UPDATE app.users SET disabled=true WHERE id=$1", testdb.Alice); err != nil {
					t.Fatal(err)
				}
				defer func() {
					_, e := fixture.Exec(ctx, "UPDATE app.users SET disabled=false WHERE id=$1", testdb.Alice)
					if e != nil {
						t.Error(e)
					}
				}()
			}
			if mode == "expired-attempt" {
				if _, err = fixture.Exec(ctx, "UPDATE app.login_attempts SET expires_at=now()-interval '1 hour'"); err != nil {
					t.Fatal(err)
				}
			}
			callback := httptest.NewRequest("GET", "https://app.example.test/auth/callback?code="+mode+"&state="+state+"&return_to=https://evil.test", nil)
			if mode != "missing-cookie" {
				// #nosec G124 -- Read-only inspection of the response cookie; its flags are asserted immediately below.
				c := start.Result().Cookies()[0]
				if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
					t.Fatal("unsafe cookie")
				}
				if mode == "wrong-state" {
					c.Value = "different"
				}
				callback.AddCookie(c)
			}
			result := httptest.NewRecorder()
			oidc.Callback(result, callback)
			expectedStatus := 401
			if mode == "valid" {
				expectedStatus = 303
			}
			if result.Code != expectedStatus {
				t.Fatalf("status=%d want=%d", result.Code, expectedStatus)
			}
			if mode == "valid" {
				if result.Header().Get("Location") != "/requests" {
					t.Fatal("unsafe return URL")
				}
				again := httptest.NewRecorder()
				oidc.Callback(again, callback)
				if again.Code != 401 {
					t.Fatal("state replay accepted")
				}
			}
		})
	}
}

func TestSessions(t *testing.T) {
	st, fixture := testdb.New(t)
	ctx := context.Background()
	service := Service{Queries: st.Auth, Secure: true}
	for _, mode := range []string{"logout", "idle", "absolute", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			w := httptest.NewRecorder()
			if err := service.Issue(ctx, w, testdb.Bob); err != nil {
				t.Fatal(err)
			}
			cookie := w.Result().Cookies()[0]
			r := httptest.NewRequest("GET", "https://app.test/requests", nil)
			r.AddCookie(cookie)
			if _, err := service.User(r); err != nil {
				t.Fatal(err)
			}
			var stored []byte
			if err := fixture.QueryRow(ctx, "SELECT token_hash FROM app.sessions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 1", testdb.Bob).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if string(stored) == cookie.Value || len(stored) != 32 {
				t.Fatal("session token stored without digest")
			}
			var err error
			switch mode {
			case "logout":
				service.Logout(httptest.NewRecorder(), r)
			case "idle":
				_, err = fixture.Exec(ctx, "UPDATE app.sessions SET touched_at=now()-interval '31 minutes' WHERE token_hash=$1", Hash(cookie.Value))
			case "absolute":
				_, err = fixture.Exec(ctx, "UPDATE app.sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", Hash(cookie.Value))
			case "disabled":
				_, err = fixture.Exec(ctx, "UPDATE app.users SET disabled=true WHERE id=$1", testdb.Bob)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.User(r); err == nil {
				t.Fatal("revoked session accepted")
			}
		})
	}
}
