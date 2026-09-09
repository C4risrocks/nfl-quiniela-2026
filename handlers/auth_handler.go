package handlers

import (
	"net/http"
	"strings"

	"nfl-quiniela-2026/services/auth"
)

type AuthHandler struct {
	authService *auth.AuthService
	renderer    *Renderer
}

func NewAuthHandler(authService *auth.AuthService, renderer *Renderer) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		renderer:    renderer,
	}
}

func (h *AuthHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if user := auth.GetUserFromContext(r.Context()); user != nil {
		http.Redirect(w, r, "/picks", http.StatusSeeOther)
		return
	}
	h.renderer.RenderPage(w, "login.html", map[string]interface{}{
		"ActiveNav": "login",
	})
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	usernameOrEmail := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	user, err := h.authService.Authenticate(usernameOrEmail, password)
	if err != nil {
		h.renderer.RenderPage(w, "login.html", map[string]interface{}{
			"ActiveNav": "login",
			"Error":     "Usuario, correo o contraseña incorrectos.",
		})
		return
	}

	h.authService.SetSessionCookie(w, user.ID)
	http.Redirect(w, r, "/picks", http.StatusSeeOther)
}

func (h *AuthHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if user := auth.GetUserFromContext(r.Context()); user != nil {
		http.Redirect(w, r, "/picks", http.StatusSeeOther)
		return
	}
	h.renderer.RenderPage(w, "register.html", map[string]interface{}{
		"ActiveNav": "register",
	})
}

func (h *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	user, err := h.authService.Register(username, email, password)
	if err != nil {
		h.renderer.RenderPage(w, "register.html", map[string]interface{}{
			"ActiveNav": "register",
			"Error":     err.Error(),
		})
		return
	}

	h.authService.SetSessionCookie(w, user.ID)
	http.Redirect(w, r, "/picks", http.StatusSeeOther)
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	h.authService.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
