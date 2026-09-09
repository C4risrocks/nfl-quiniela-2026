package handlers

import (
	"net/http"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
)

type RulesHandler struct {
	repo     *db.Repository
	renderer *Renderer
}

func NewRulesHandler(repo *db.Repository, renderer *Renderer) *RulesHandler {
	return &RulesHandler{
		repo:     repo,
		renderer: renderer,
	}
}

func (h *RulesHandler) ShowRules(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	scoringCfg, _ := h.repo.GetScoringConfig()

	h.renderer.RenderPage(w, "rules.html", map[string]interface{}{
		"ActiveNav":     "rules",
		"User":          user,
		"ScoringConfig": scoringCfg,
	})
}
