package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"harbor/internal/store"
	"harbor/internal/telegram"

	"github.com/go-chi/chi/v5"
)

// telegramContext outlives the browser connection being closed, and it ends
// before a proxy such as Cloudflare replaces a slow response with an HTML page.
func telegramContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), 50*time.Second)
}

func (a *API) telegramSendCode(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	ctx, cancel := telegramContext(r)
	defer cancel()
	status, err := client.SendCode(ctx)
	if saveErr := a.store.UpdateDestinationSecret(ctx, dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("telegram sentCode destination=%s type=%s next_type=%s timeout=%d dc=%d", dest.ID, status.TypeName, status.NextTypeName, status.Timeout, status.DC)
	a.store.AddAudit(ctx, principalFrom(r).Actor(), "destination.telegram_code", dest.ID, dest.Name)
	writeJSON(w, http.StatusOK, status)
}

func (a *API) telegramResendCode(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	ctx, cancel := telegramContext(r)
	defer cancel()
	status, err := client.ResendCode(ctx)
	if saveErr := a.store.UpdateDestinationSecret(ctx, dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("telegram resendCode destination=%s type=%s next_type=%s timeout=%d dc=%d", dest.ID, status.TypeName, status.NextTypeName, status.Timeout, status.DC)
	writeJSON(w, http.StatusOK, status)
}

func (a *API) telegramCancelCode(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	ctx, cancel := telegramContext(r)
	defer cancel()
	err := client.CancelCode(ctx)
	if saveErr := a.store.UpdateDestinationSecret(ctx, dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) telegramSendEmail(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := telegramContext(r)
	defer cancel()
	pattern, length, err := client.SendLoginEmail(ctx, body.Email)
	if saveErr := a.store.UpdateDestinationSecret(ctx, dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email_pattern": pattern, "length": length})
}

func (a *API) telegramVerifyEmail(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := telegramContext(r)
	defer cancel()
	status, err := client.VerifyLoginEmail(ctx, body.Code)
	if saveErr := a.store.UpdateDestinationSecret(ctx, dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("telegram verifyEmail destination=%s type=%s next_type=%s timeout=%d dc=%d", dest.ID, status.TypeName, status.NextTypeName, status.Timeout, status.DC)
	writeJSON(w, http.StatusOK, status)
}

func (a *API) telegramSignIn(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	var body struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := telegramContext(r)
	defer cancel()
	code := strings.TrimSpace(body.Code)
	password := strings.TrimSpace(body.Password)
	var name string
	var err error
	switch {
	case password != "" && code == "":
		name, err = client.Password(ctx, password)
	case code != "":
		name, err = client.SignIn(ctx, code)
		if errors.Is(err, telegram.ErrPasswordNeeded) {
			if password == "" {
				if saveErr := a.store.UpdateDestinationSecret(ctx, dest.ID, client.Secret()); saveErr != nil {
					writeError(w, http.StatusInternalServerError, "could not save the telegram session")
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"status": "password"})
				return
			}
			name, err = client.Password(ctx, password)
		}
	default:
		writeError(w, http.StatusBadRequest, "enter the login code")
		return
	}
	if saveErr := a.store.UpdateDestinationSecret(ctx, dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	a.store.AddAudit(ctx, principalFrom(r).Actor(), "destination.telegram_login", dest.ID, name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "account_label": name})
}

func (a *API) telegramLogout(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	ctx, cancel := telegramContext(r)
	defer cancel()
	err := client.LogOut(ctx)
	if saveErr := a.store.UpdateDestinationSecret(ctx, dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	a.store.AddAudit(ctx, principalFrom(r).Actor(), "destination.telegram_logout", dest.ID, dest.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) telegramClient(w http.ResponseWriter, r *http.Request) (store.Destination, *telegram.Client, bool) {
	if !requireOwner(w, r) {
		return store.Destination{}, nil, false
	}
	dest, err := a.store.GetDestination(r.Context(), chi.URLParam(r, "destinationID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "destination not found")
		return store.Destination{}, nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load destination")
		return store.Destination{}, nil, false
	}
	if dest.Provider != store.ProviderTelegram {
		writeError(w, http.StatusBadRequest, "this destination is not Telegram")
		return store.Destination{}, nil, false
	}
	client, err := telegram.Open(dest)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return store.Destination{}, nil, false
	}
	return dest, client, true
}
