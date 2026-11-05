package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"taskforge/internal/auth"
	"taskforge/internal/repository"
)

// AuthService defines the authentication operations required by HTTP handlers.
type AuthService interface {
	Register(
		ctx context.Context,
		email string,
		password string,
		tenantName string,
	) (auth.AuthResult, error)

	Login(
		ctx context.Context,
		email string,
		password string,
		tenantID string,
	) (auth.AuthResult, error)

	Refresh(
		ctx context.Context,
		refreshToken string,
	) (auth.AuthResult, error)

	Logout(
		ctx context.Context,
		refreshToken string,
	) error
}

type RegisterRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TenantName string `json:"tenant_name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TenantID string `json:"tenant_id"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	UserID       string `json:"user_id"`
	Email        string `json:"email"`
	TenantID     string `json:"tenant_id"`
	Role         string `json:"role"`
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// register handles user registration.
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest

	if err := decodeJSON(w, r, &req); err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Invalid JSON request",
		)
		return
	}

	result, err := h.auth.Register(
		r.Context(),
		req.Email,
		req.Password,
		req.TenantName,
	)

	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			writeError(
				w,
				http.StatusBadRequest,
				"INVALID_INPUT",
				"Invalid registration details",
			)

		case errors.Is(err, auth.ErrUserExists):
			writeError(
				w,
				http.StatusConflict,
				"USER_ALREADY_EXISTS",
				"An account with this email already exists",
			)

		default:
			writeError(
				w,
				http.StatusInternalServerError,
				"REGISTRATION_FAILED",
				"Registration failed",
			)
		}
		return
	}

	writeJSON(w, http.StatusCreated, toAuthResponse(result))

}

// login handles user authentication.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := decodeJSON(w, r, &req); err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Invalid JSON request",
		)
		return
	}

	result, err := h.auth.Login(
		r.Context(),
		req.Email,
		req.Password,
		req.TenantID,
	)

	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials),
			errors.Is(err, auth.ErrInactiveUser),
			errors.Is(err, auth.ErrInvalidMembership):

			writeError(
				w,
				http.StatusUnauthorized,
				"INVALID_CREDENTIALS",
				"Invalid email or password",
			)

		default:
			writeError(
				w,
				http.StatusInternalServerError,
				"LOGIN_FAILED",
				"Login could not be completed",
			)
		}
		return
	}

	writeJSON(w, http.StatusOK, toAuthResponse(result))

}

// refresh exchanges a refresh token for a new token pair.
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest

	if err := decodeJSON(w, r, &req); err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Invalid JSON request",
		)
		return
	}

	result, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		if isInvalidRefreshTokenError(err) {
			writeError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Refresh token is invalid or expired")
		} else {
			writeError(w, http.StatusInternalServerError, "REFRESH_FAILED", "Token refresh could not be completed")
		}
		return
	}

	writeJSON(w, http.StatusOK, toAuthResponse(result))

}

// logout revokes the refresh-token family.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req LogoutRequest

	if err := decodeJSON(w, r, &req); err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Invalid JSON request",
		)
		return
	}

	if err := h.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		if errors.Is(err, repository.ErrRefreshTokenInvalid) {
			writeError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Refresh token is invalid")
		} else {
			writeError(w, http.StatusInternalServerError, "LOGOUT_FAILED", "Logout could not be completed")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})

}

func toAuthResponse(result auth.AuthResult) AuthResponse {
	return AuthResponse{
		UserID:       result.UserID,
		Email:        result.Email,
		TenantID:     result.TenantID,
		Role:         result.Role,
		AccessToken:  result.Token,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(auth.AccessTokenTTL.Seconds()),
	}
}

func isInvalidRefreshTokenError(err error) bool {
	return errors.Is(err, auth.ErrInvalidCredentials) ||
		errors.Is(err, repository.ErrRefreshTokenInvalid) ||
		errors.Is(err, repository.ErrRefreshTokenExpired) ||
		errors.Is(err, repository.ErrRefreshTokenReuse) ||
		errors.Is(err, repository.ErrRefreshTokenInactive) ||
		errors.Is(err, repository.ErrRefreshTokenMembership)
}

// decodeJSON validates and decodes exactly one JSON request object.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(
		r.Header.Get("Content-Type"), ";",
	)[0]))

	if mediaType != "application/json" {
		return errors.New("content type must be application/json")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON object")
	}

	return nil

}
