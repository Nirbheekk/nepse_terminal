package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"nepse/terminal/api/internal/auth"
	"nepse/terminal/api/internal/repositories"
)

type credentials struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// validateCredentials applies the same rules the browser form applies, but
// server-side: a client-side check is a convenience, never a control.
func validateCredentials(c credentials, requireName bool) string {
	email := strings.TrimSpace(c.Email)
	if len(email) > 254 {
		return "email is too long"
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email, ".") {
		return "email is not valid"
	}
	if len(c.Password) < 10 {
		return "password must be at least 10 characters"
	}
	if len(c.Password) > 128 {
		return "password must be at most 128 characters"
	}
	if requireName && strings.TrimSpace(c.DisplayName) == "" {
		return "display_name is required"
	}
	return ""
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decodeJSON(r, &in); err != nil {
		badRequest(w, r, "invalid JSON body")
		return
	}
	if msg := validateCredentials(in, true); msg != "" {
		Fail(w, r, http.StatusUnprocessableEntity, CodeValidation, msg)
		return
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.internal(w, r, "hash password", err)
		return
	}
	user, err := s.store.CreateUser(r.Context(), strings.TrimSpace(in.Email), hash, strings.TrimSpace(in.DisplayName))
	if err != nil {
		if errors.Is(err, repositories.ErrEmailTaken) {
			Fail(w, r, http.StatusConflict, CodeConflict, "email already registered")
			return
		}
		s.internal(w, r, "create user", err)
		return
	}

	_ = s.store.Audit(r.Context(), user.ID, clientIP(r), "register", user.Email, "success", RequestIDFrom(r.Context()), nil)
	s.startSession(w, r, user, http.StatusCreated)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decodeJSON(r, &in); err != nil {
		badRequest(w, r, "invalid JSON body")
		return
	}
	email := strings.TrimSpace(in.Email)
	if email == "" || in.Password == "" {
		badRequest(w, r, "email and password are required")
		return
	}

	user, hash, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		// Verify against a dummy hash anyway so a missing account and a wrong
		// password take comparable time.
		_, _ = auth.VerifyPassword(s.dummyPwd, in.Password)
		_ = s.store.Audit(r.Context(), "", clientIP(r), "login", email, "unknown_account", RequestIDFrom(r.Context()), nil)
		Fail(w, r, http.StatusUnauthorized, CodeUnauthenticated, "invalid email or password")
		return
	}
	if user.Status != "active" {
		Fail(w, r, http.StatusForbidden, CodeForbidden, "account is not active")
		return
	}

	ok, err := auth.VerifyPassword(hash, in.Password)
	if err != nil || !ok {
		_ = s.store.Audit(r.Context(), user.ID, clientIP(r), "login", email, "bad_password", RequestIDFrom(r.Context()), nil)
		Fail(w, r, http.StatusUnauthorized, CodeUnauthenticated, "invalid email or password")
		return
	}

	if err := s.store.TouchLogin(r.Context(), user.ID); err != nil {
		s.logger.Warn("touch login", "error", err)
	}
	_ = s.store.Audit(r.Context(), user.ID, clientIP(r), "login", email, "success", RequestIDFrom(r.Context()), nil)
	s.startSession(w, r, user, http.StatusOK)
}

// startSession creates the refresh session, issues the access token and sets all
// three cookies. It is shared by register and login.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user *repositories.User, status int) {
	refreshRaw, refreshHash, err := auth.NewOpaqueToken()
	if err != nil {
		s.internal(w, r, "refresh token", err)
		return
	}
	csrf, err := auth.RandomHex(24)
	if err != nil {
		s.internal(w, r, "csrf token", err)
		return
	}
	expires := time.Now().UTC().Add(s.cfg.RefreshTTL)
	if _, _, err := s.store.CreateSession(r.Context(), user.ID, refreshHash, expires,
		r.UserAgent(), clientIP(r)); err != nil {
		s.internal(w, r, "create session", err)
		return
	}

	access, accessExp, err := s.tokens.Issue(user.ID, user.Roles)
	if err != nil {
		s.internal(w, r, "issue access", err)
		return
	}
	s.setCookies(w, access, accessExp, refreshRaw, expires, csrf)
	Succeed(w, r, status, user, Meta{RequestID: RequestIDFrom(r.Context())})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		unauthenticated(w, r)
		return
	}
	hash := auth.HashSecret(c.Value)
	sess, userID, err := s.store.SessionByHash(r.Context(), hash)
	if err != nil {
		unauthenticated(w, r)
		return
	}
	if sess.RevokedAt != nil {
		// A revoked token was presented again: treat the whole family as
		// compromised rather than merely rejecting this request.
		_ = s.store.RevokeFamily(r.Context(), sess.FamilyID)
		_ = s.store.Audit(r.Context(), userID, clientIP(r), "refresh_reuse", sess.FamilyID, "denied", RequestIDFrom(r.Context()), nil)
		s.clearCookies(w)
		Fail(w, r, http.StatusUnauthorized, CodeUnauthenticated, "session expired, please sign in again")
		return
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		s.clearCookies(w)
		unauthenticated(w, r)
		return
	}

	user, err := s.store.GetUser(r.Context(), userID)
	if err != nil || user.Status != "active" {
		s.clearCookies(w)
		unauthenticated(w, r)
		return
	}

	refreshRaw, refreshHash, err := auth.NewOpaqueToken()
	if err != nil {
		s.internal(w, r, "refresh token", err)
		return
	}
	csrf, err := auth.RandomHex(24)
	if err != nil {
		s.internal(w, r, "csrf token", err)
		return
	}
	expires := time.Now().UTC().Add(s.cfg.RefreshTTL)
	if _, err := s.store.RotateSession(r.Context(), sess.ID, userID, sess.FamilyID, refreshHash, expires,
		r.UserAgent(), clientIP(r)); err != nil {
		s.internal(w, r, "rotate session", err)
		return
	}

	access, accessExp, err := s.tokens.Issue(user.ID, user.Roles)
	if err != nil {
		s.internal(w, r, "issue access", err)
		return
	}
	s.setCookies(w, access, accessExp, refreshRaw, expires, csrf)
	OK(w, r, user, metaFrom(r))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil && c.Value != "" {
		if err := s.store.RevokeSessionByHash(r.Context(), auth.HashSecret(c.Value)); err != nil {
			s.logger.Warn("revoke session", "error", err)
		}
	}
	if claims, ok := ClaimsFrom(r.Context()); ok {
		_ = s.store.Audit(r.Context(), claims.Subject, clientIP(r), "logout", claims.Subject, "success", RequestIDFrom(r.Context()), nil)
	}
	s.clearCookies(w)
	OK(w, r, map[string]bool{"signed_out": true}, metaFrom(r))
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFrom(r.Context())
	user, err := s.store.GetUser(r.Context(), claims.Subject)
	if err != nil {
		unauthenticated(w, r)
		return
	}
	OK(w, r, user, metaFrom(r))
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFrom(r.Context())
	list, err := s.store.ListSessions(r.Context(), claims.Subject)
	if err != nil {
		s.internal(w, r, "list sessions", err)
		return
	}
	OK(w, r, list, metaFrom(r))
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFrom(r.Context())
	id := pathParam(r, "id")
	if id == "" {
		badRequest(w, r, "session id is required")
		return
	}
	if err := s.store.RevokeSession(r.Context(), id, claims.Subject); err != nil {
		s.internal(w, r, "revoke session", err)
		return
	}
	_ = s.store.Audit(r.Context(), claims.Subject, clientIP(r), "session_revoke", id, "success", RequestIDFrom(r.Context()), nil)
	OK(w, r, map[string]bool{"revoked": true}, metaFrom(r))
}

// ---------------------------------------------------------------- cookies

func (s *Server) setCookies(w http.ResponseWriter, access string, accessExp time.Time, refresh string, refreshExp time.Time, csrf string) {
	http.SetCookie(w, &http.Cookie{
		Name: accessCookie, Value: access, Path: "/", Domain: s.cfg.CookieDomain,
		Expires: accessExp, MaxAge: int(s.cfg.AccessTTL.Seconds()),
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: refresh, Path: "/api/v1/auth", Domain: s.cfg.CookieDomain,
		Expires: refreshExp, MaxAge: int(s.cfg.RefreshTTL.Seconds()),
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	// The CSRF cookie is intentionally readable by the page: that is what makes
	// double-submit work. It is not a credential by itself.
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookie, Value: csrf, Path: "/", Domain: s.cfg.CookieDomain,
		Expires: refreshExp, MaxAge: int(s.cfg.RefreshTTL.Seconds()),
		HttpOnly: false, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookies(w http.ResponseWriter) {
	for _, c := range []struct{ name, path string }{
		{accessCookie, "/"},
		{refreshCookie, "/api/v1/auth"},
		{csrfCookie, "/"},
	} {
		http.SetCookie(w, &http.Cookie{
			Name: c.name, Value: "", Path: c.path, Domain: s.cfg.CookieDomain,
			MaxAge: -1, Expires: time.Unix(0, 0),
			HttpOnly: c.name != csrfCookie, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
		})
	}
}
