package server

import "net/http"

// Expose only administrator-approved text models, never upstream credentials.
func (s *Server) chatModels(w http.ResponseWriter, r *http.Request) {
	if s.config.Chat.URL == "" {
		fail(w, 503, "feature_disabled")
		return
	}
	type model struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	models := []model{{s.config.Chat.Model, "model"}}
	for _, id := range s.config.Chat.Models {
		if id != s.config.Chat.Model {
			models = append(models, model{id, "model"})
		}
	}
	respond(w, 200, map[string]any{"object": "list", "data": models, "default_model": s.config.Chat.Model})
}
