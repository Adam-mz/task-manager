package user

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"task-manager/internal/auth"
	"time"
)

type HTTPHandler struct {
	service *Service
	tokens  *auth.TokenManager
}

func NewHTTPHandler(service *Service, tokens *auth.TokenManager) *HTTPHandler {
	return &HTTPHandler{
		service: service,
		tokens:  tokens,
	}
}

func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", h.Register)
	mux.HandleFunc("POST /auth/login", h.Login)
	return mux
}

func (h *HTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request loginRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeBodyDecodeError(w, err)
		return
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeBodyDecodeError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	found, err := h.service.Authenticate(ctx, request.Email, request.Password)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return

		case errors.Is(err, context.DeadlineExceeded):
			_ = writeError(w, http.StatusServiceUnavailable, "request timed out")

		case errors.Is(err, ErrInvalidCredentials):
			_ = writeError(w, http.StatusUnauthorized, ErrInvalidCredentials.Error())

		default:
			_ = writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	accessToken, err := h.tokens.Generate(found.ID)
	if err != nil {
		log.Printf("generate token for user %d: %v", found.ID, err)
		_ = writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	_ = writeJSON(w, http.StatusOK, loginResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
	})
}

func (h *HTTPHandler) Register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var register registerRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&register); err != nil {
		writeBodyDecodeError(w, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeBodyDecodeError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	created, err := h.service.Register(ctx, register.Email, register.Password)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return

		case errors.Is(err, context.DeadlineExceeded):
			_ = writeError(w, http.StatusServiceUnavailable, "request timed out")

		case errors.Is(err, ErrInvalidEmail):
			_ = writeError(w, http.StatusBadRequest, ErrInvalidEmail.Error())

		case errors.Is(err, ErrInvalidPassword):
			_ = writeError(w, http.StatusBadRequest, ErrInvalidPassword.Error())

		case errors.Is(err, ErrEmailAlreadyExists):
			_ = writeError(w, http.StatusConflict, ErrEmailAlreadyExists.Error())

		default:
			_ = writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	_ = writeJSON(w, http.StatusCreated, userResponse{
		ID:        created.ID,
		Email:     created.Email,
		CreatedAt: created.CreatedAt,
	})

}
func writeBodyDecodeError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		_ = writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	_ = writeError(w, http.StatusBadRequest, "invalid request body")
}
