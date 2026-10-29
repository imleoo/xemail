package handler

import (
	"net/http"

	"xemail/internal/service"
)

type domainsHandler struct {
	svc *service.Service
}

func (h *domainsHandler) list(w http.ResponseWriter, _ *http.Request) {
	domains, err := h.svc.ListDomains()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": domains})
}

func (h *domainsHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	d, err := h.svc.AddDomain(req.Name)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"domain": d})
}

func (h *domainsHandler) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteDomain(r.PathValue("name")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
