package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"harbor/internal/store"
	"harbor/internal/telegram"

	"github.com/go-chi/chi/v5"
)

func (a *API) telegramSendCode(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	delivery, err := client.SendCode(r.Context())
	if saveErr := a.store.UpdateDestinationSecret(r.Context(), dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	a.store.AddAudit(r.Context(), principalFrom(r).Actor(), "destination.telegram_code", dest.ID, dest.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "code", "delivery": delivery})
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
	code := strings.TrimSpace(body.Code)
	password := strings.TrimSpace(body.Password)
	var name string
	var err error
	switch {
	case password != "" && code == "":
		name, err = client.Password(r.Context(), password)
	case code != "":
		name, err = client.SignIn(r.Context(), code)
		if errors.Is(err, telegram.ErrPasswordNeeded) {
			if password == "" {
				if saveErr := a.store.UpdateDestinationSecret(r.Context(), dest.ID, client.Secret()); saveErr != nil {
					writeError(w, http.StatusInternalServerError, "could not save the telegram session")
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"status": "password"})
				return
			}
			name, err = client.Password(r.Context(), password)
		}
	default:
		writeError(w, http.StatusBadRequest, "enter the login code")
		return
	}
	if saveErr := a.store.UpdateDestinationSecret(r.Context(), dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	a.store.AddAudit(r.Context(), principalFrom(r).Actor(), "destination.telegram_login", dest.ID, name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "account_label": name})
}

func (a *API) telegramLogout(w http.ResponseWriter, r *http.Request) {
	dest, client, ok := a.telegramClient(w, r)
	if !ok {
		return
	}
	err := client.LogOut(r.Context())
	if saveErr := a.store.UpdateDestinationSecret(r.Context(), dest.ID, client.Secret()); saveErr != nil {
		writeError(w, http.StatusInternalServerError, "could not save the telegram session")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	a.store.AddAudit(r.Context(), principalFrom(r).Actor(), "destination.telegram_logout", dest.ID, dest.Name)
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
