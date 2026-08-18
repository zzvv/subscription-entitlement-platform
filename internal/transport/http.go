package transport

import (
	"encoding/json"
	"example.com/subscription-entitlement-platform/internal/application"
	"example.com/subscription-entitlement-platform/internal/domain"
	"net/http"
)

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
func (h *Handler) Process(w http.ResponseWriter, r *http.Request) {
	var command domain.Command
	if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	value, err := h.service.Process(r.Context(), command)
	if err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	w.WriteHeader(202)
	_ = json.NewEncoder(w).Encode(value)
}
func Routes(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Health)
	mux.HandleFunc("POST /v1/subscriptions/commands", h.Process)
	return mux
}
