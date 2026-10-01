package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/opensave/opensave/internal/presets"
)

func (s *Server) handleTrainerName(w http.ResponseWriter, r *http.Request) {
	game, err := s.Daemon.Store.GetGame(chi.URLParam(r, "gameId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": presets.ResolveTrainerName(r.Context(), game.AppID, game.Name)})
}
