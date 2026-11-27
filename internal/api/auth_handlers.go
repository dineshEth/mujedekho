package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"log/slog"

	"site24x7/internal/auth"
	"site24x7/internal/domain"
	"site24x7/internal/store"
)

var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// AuthHandler implements registration, login and refresh-token issuance.
type AuthHandler struct {
	All    *store.All
	Tokens *auth.Tokens
	Log    *slog.Logger
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type authResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresIn   int          `json:"expires_in"` // seconds
	User        *domain.User `json:"user"`
}

// Register creates a user; no auto-O org — organization creation is explicit
// (POST /api/v1/organizations) so invitation flows stay possible.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Malformed JSON body.", nil)
		return
	}
	if errs := validateRegister(req); len(errs) > 0 {
		WriteError(w, h.Log, http.StatusUnprocessableEntity, "validation_failed", "Invalid registration payload.", errs)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		WriteInternal(w, h.Log, err, "hash password")
		return
	}
	user := &domain.User{
		Email:        auth.NormalizeEmail(req.Email),
		PasswordHash: hash,
		FullName:     strings.TrimSpace(req.FullName),
	}
	if err := h.All.Users.Create(r.Context(), user); err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "create user")
		}
		return
	}
	h.issueTokens(w, r, user, http.StatusCreated)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login verifies credentials; timing-equalized failure responses.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Malformed JSON body.", nil)
		return
	}
	user, err := h.All.Users.ByEmail(r.Context(), auth.NormalizeEmail(req.Email))
	if err != nil {
		// Burn comparable time so absence leaks nothing through latency.
		_ = auth.VerifyPassword(req.Password, "$argon2id$v=19$m=65536,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		WriteError(w, h.Log, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.", nil)
		return
	}
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		WriteError(w, h.Log, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.", nil)
		return
	}
	h.issueTokens(w, r, user, http.StatusOK)
}

func (h *AuthHandler) issueTokens(w http.ResponseWriter, r *http.Request, user *domain.User, status int) {
	token, err := h.Tokens.SignAccess(user.ID, user.Email)
	if err != nil {
		WriteInternal(w, h.Log, err, "sign access token")
		return
	}
	writeJSON(w, h.Log, status, authResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(h.Tokens.AccessTTL().Seconds()),
		User:        user,
	})
}

func validateRegister(req registerRequest) map[string]any {
	errs := map[string]any{}
	email := auth.NormalizeEmail(req.Email)
	if email == "" || !emailRe.MatchString(email) || len(email) > 254 {
		errs["email"] = "must be a valid email address"
	}
	if req.Password == "" {
		errs["password"] = "is required"
	}
	if n := utf8.RuneCountInString(req.Password); n > 0 && n < 12 {
		errs["password"] = "must be at least 12 characters"
	}
	if utf8.RuneCountInString(req.FullName) > 100 {
		errs["full_name"] = "must be at most 100 characters"
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Me returns the authenticated user plus organization memberships.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	id, ok := IdentityFrom(r.Context())
	if !ok {
		WriteError(w, h.Log, http.StatusUnauthorized, "unauthorized", "Authentication required.", nil)
		return
	}
	user, err := h.All.Users.Get(r.Context(), id.UserID)
	if err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "get user")
		}
		return
	}
	orgs, err := h.All.Organizations.ListForUser(r.Context(), id.UserID)
	if err != nil {
		WriteInternal(w, h.Log, err, "list orgs for user")
		return
	}
	if orgs == nil {
		orgs = []domain.Organization{} // new users: [] not null, clients iterate it
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{"user": user, "organizations": orgs})
}

// BodyUnknownFields decodes strictly and returns a validation map on failure.
func BodyUnknownFields(r *http.Request, dst any) (map[string]any, bool) {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return map[string]any{"body": "malformed JSON or unknown field: " + err.Error()}, false
	}
	return nil, true
}

func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error("api: encode response", "err", err)
	}
}
