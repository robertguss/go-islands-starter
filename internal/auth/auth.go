package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/robertguss/go-islands-starter/internal/db"
	"golang.org/x/oauth2"
)

const GoogleIssuer = "https://accounts.google.com"

type Service struct {
	Queries *db.Queries
	Secure  bool
}
type OIDC struct {
	Sessions Service
	OAuth    oauth2.Config
	Verifier *oidc.IDTokenVerifier
	Domains  []string
}

func Hash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
func (s Service) cookieName() string {
	if s.Secure {
		return "__Host-islands"
	}
	return "islands"
}
func (s Service) cookie(w http.ResponseWriter, name, value string, maxAge int) {
	// #nosec G124 -- Production always sets Secure=true; only the isolated loopback preview uses HTTP.
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func (s Service) Issue(ctx context.Context, w http.ResponseWriter, userID string) error {
	token := rand.Text()
	if err := s.Queries.CreateSession(ctx, db.CreateSessionParams{TokenHash: Hash(token), UserID: userID}); err != nil {
		return err
	}
	s.cookie(w, s.cookieName(), token, 8*60*60)
	return nil
}
func (s Service) User(r *http.Request) (db.AppUser, error) {
	cookie, err := r.Cookie(s.cookieName())
	if err != nil {
		return db.AppUser{}, errors.New("sign in required")
	}
	user, err := s.Queries.SessionUser(r.Context(), Hash(cookie.Value))
	if err != nil {
		return db.AppUser{}, errors.New("sign in required")
	}
	if err = s.Queries.TouchSession(r.Context(), Hash(cookie.Value)); err != nil {
		return db.AppUser{}, errors.New("session unavailable")
	}
	return user, nil
}
func (s Service) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(s.cookieName()); err == nil {
		if err = s.Queries.DeleteSession(r.Context(), Hash(cookie.Value)); err != nil {
			http.Error(w, "Unable to sign out. Please retry.", http.StatusServiceUnavailable)
			return
		}
	}
	s.cookie(w, s.cookieName(), "", -1)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func NewOIDC(ctx context.Context, s Service, clientID, secret, origin string, domains []string) (*OIDC, error) {
	return newOIDC(ctx, s, clientID, secret, origin, domains, GoogleIssuer)
}

func newOIDC(ctx context.Context, s Service, clientID, secret, origin string, domains []string, issuer string) (*OIDC, error) {
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 10 * time.Second})
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, errors.New("identity provider unavailable")
	}
	return &OIDC{Sessions: s, OAuth: oauth2.Config{ClientID: clientID, ClientSecret: secret, Endpoint: provider.Endpoint(), RedirectURL: origin + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "email"}}, Verifier: provider.Verifier(&oidc.Config{ClientID: clientID}), Domains: domains}, nil
}

func (o *OIDC) Start(w http.ResponseWriter, r *http.Request) {
	state, nonce, verifier := rand.Text(), rand.Text(), oauth2.GenerateVerifier()
	err := o.Sessions.Queries.CreateLoginAttempt(r.Context(), db.CreateLoginAttemptParams{StateHash: Hash(state), Nonce: nonce, Verifier: verifier})
	if err != nil {
		http.Error(w, "Sign-in is temporarily unavailable.", http.StatusServiceUnavailable)
		return
	}
	o.Sessions.cookie(w, o.Sessions.cookieName()+"-state", state, 600)
	http.Redirect(w, r, o.OAuth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (o *OIDC) Callback(w http.ResponseWriter, r *http.Request) {
	fail := func() {
		http.Error(w, "Sign-in was not accepted. Start again or contact your administrator.", http.StatusUnauthorized)
	}
	cookie, err := r.Cookie(o.Sessions.cookieName() + "-state")
	state := r.URL.Query().Get("state")
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		fail()
		return
	}
	o.Sessions.cookie(w, o.Sessions.cookieName()+"-state", "", -1)
	attempt, err := o.Sessions.Queries.ConsumeLoginAttempt(r.Context(), Hash(state))
	if err != nil {
		fail()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 10 * time.Second})
	token, err := o.OAuth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(attempt.Verifier))
	if err != nil {
		fail()
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		fail()
		return
	}
	verified, err := o.Verifier.Verify(ctx, raw)
	if err != nil {
		fail()
		return
	}
	if subtle.ConstantTimeCompare([]byte(verified.Nonce), []byte(attempt.Nonce)) != 1 {
		fail()
		return
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Domain   string `json:"hd"`
	}
	if err = verified.Claims(&claims); err != nil || !claims.Verified || !AllowedDomain(claims.Domain, o.Domains) || verified.Subject == "" {
		fail()
		return
	}
	user, err := o.Sessions.Queries.LinkIdentity(ctx, db.LinkIdentityParams{Subject: verified.Issuer + "|" + verified.Subject, Email: strings.ToLower(claims.Email)})
	if err != nil {
		fail()
		return
	}
	// Delete an existing browser session before replacing it to prevent fixation.
	if old, e := r.Cookie(o.Sessions.cookieName()); e == nil {
		if e = o.Sessions.Queries.DeleteSession(ctx, Hash(old.Value)); e != nil {
			fail()
			return
		}
	}
	if err = o.Sessions.Issue(ctx, w, user.ID); err != nil {
		fail()
		return
	}
	http.Redirect(w, r, "/requests", http.StatusSeeOther)
}

func AllowedDomain(domain string, allowed []string) bool {
	if domain == "" {
		return false
	}
	for _, candidate := range allowed {
		if strings.EqualFold(domain, candidate) {
			return true
		}
	}
	return false
}
