package handler

import (
	"net/http"
	"strconv"

	"xemail/internal/service"
)

type accountsHandler struct {
	svc *service.Service
}

func (h *accountsHandler) list(w http.ResponseWriter, _ *http.Request) {
	accounts, err := h.svc.ListAccounts()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

func (h *accountsHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
		Note    string `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	acct, password, err := h.svc.CreateAccount(req.Address, req.Note)
	if err != nil {
		fail(w, err)
		return
	}
	writeSecret(w, http.StatusCreated, map[string]any{"account": acct, "password": password})
}

func (h *accountsHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	password, err := h.svc.ResetPassword(id)
	if err != nil {
		fail(w, err)
		return
	}
	writeSecret(w, http.StatusOK, map[string]string{"password": password})
}

func (h *accountsHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Enabled *bool   `json:"enabled"`
		Note    *string `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	acct, err := h.svc.UpdateAccount(id, req.Enabled, req.Note)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": acct})
}

func (h *accountsHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteAccount(id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "账号 id 不合法")
		return 0, false
	}
	return id, true
}
