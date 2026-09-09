package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/notifications"
)

type AuthHandler struct {
	authService *auth.AuthService
	repo        *db.Repository
	emailSender *notifications.EmailSender
	renderer    *Renderer
}

func NewAuthHandler(authService *auth.AuthService, repo *db.Repository, emailSender *notifications.EmailSender, renderer *Renderer) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		repo:        repo,
		emailSender: emailSender,
		renderer:    renderer,
	}
}

func (h *AuthHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if user := auth.GetUserFromContext(r.Context()); user != nil {
		http.Redirect(w, r, "/picks", http.StatusSeeOther)
		return
	}

	successMsg := ""
	if r.URL.Query().Get("reset_success") == "1" {
		successMsg = "Tu contraseña ha sido restablecida exitosamente. Inicia sesión con tus nuevas credenciales."
	}

	h.renderer.RenderPage(w, "login.html", map[string]interface{}{
		"ActiveNav": "login",
		"Success":   successMsg,
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

	// Generate secure 32-byte (64 hex characters) email verification token
	token, err := auth.GenerateRandomToken(32)
	if err != nil {
		token = ""
	}

	user, err := h.authService.RegisterWithVerification(username, email, password, token)
	if err != nil {
		h.renderer.RenderPage(w, "register.html", map[string]interface{}{
			"ActiveNav": "register",
			"Error":     err.Error(),
		})
		return
	}

	// Dispatch email verification link in background via Gmail SMTP / Mock
	if h.emailSender != nil && token != "" {
		go func(u *db.User, tok string) {
			if err := h.emailSender.SendVerificationEmail(u, tok); err != nil {
				log.Printf("[Auth] Error sending verification email to %s: %v", u.Email, err)
			}
		}(user, token)
	}

	h.authService.SetSessionCookie(w, user.ID)
	http.Redirect(w, r, "/picks?welcome=1", http.StatusSeeOther)
}

func (h *AuthHandler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		h.renderer.RenderPage(w, "verify_status.html", map[string]interface{}{
			"Success": false,
			"Message": "No se proporcionó un token de verificación.",
		})
		return
	}

	user, err := h.repo.VerifyUserEmail(token)
	if err != nil {
		h.renderer.RenderPage(w, "verify_status.html", map[string]interface{}{
			"Success": false,
			"Message": "El enlace de verificación no es válido o ya fue utilizado.",
		})
		return
	}

	h.renderer.RenderPage(w, "verify_status.html", map[string]interface{}{
		"Success":  true,
		"Username": user.Username,
		"Message":  "¡Tu correo electrónico ha sido verificado con éxito! Ahora recibirás alertas y recordatorios de jornada.",
	})
}

func (h *AuthHandler) HandleResendVerification(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "No autenticado", http.StatusUnauthorized)
		return
	}

	// Fetch fresh user state from DB
	freshUser, err := h.repo.GetUserByID(user.ID)
	if err != nil || freshUser == nil {
		http.Error(w, "Usuario no encontrado", http.StatusInternalServerError)
		return
	}

	if freshUser.EmailVerified {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div class="p-2 rounded bg-emerald-500/10 border border-emerald-500/20 text-emerald-300 text-xs flex items-center space-x-1.5"><i class="fa-solid fa-circle-check"></i><span>Tu correo ya está verificado.</span></div>`)
		return
	}

	token, err := auth.GenerateRandomToken(32)
	if err != nil {
		http.Error(w, "Error generando token", http.StatusInternalServerError)
		return
	}

	if err := h.repo.SetVerificationToken(freshUser.ID, token); err != nil {
		http.Error(w, "Error guardando token", http.StatusInternalServerError)
		return
	}

	if h.emailSender != nil {
		go func(u *db.User, tok string) {
			_ = h.emailSender.SendVerificationEmail(u, tok)
		}(freshUser, token)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="p-2 rounded bg-emerald-500/10 border border-emerald-500/20 text-emerald-300 text-xs flex items-center space-x-1.5"><i class="fa-solid fa-paper-plane"></i><span>¡Enlace reenviado a <strong>%s</strong>! Revisa tu bandeja de entrada o spam.</span></div>`, freshUser.Email)
}

func (h *AuthHandler) ShowForgotPassword(w http.ResponseWriter, r *http.Request) {
	h.renderer.RenderPage(w, "forgot_password.html", map[string]interface{}{
		"ActiveNav": "login",
	})
}

func (h *AuthHandler) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	token, _ := auth.GenerateRandomToken(32)
	expiresAt := time.Now().Add(60 * time.Minute)

	user, err := h.repo.SetPasswordResetToken(email, token, expiresAt)
	if err == nil && user != nil && h.emailSender != nil {
		go func(u *db.User, tok string) {
			_ = h.emailSender.SendPasswordResetEmail(u, tok)
		}(user, token)
	}

	// Always show neutral success message for security (prevent email discovery)
	h.renderer.RenderPage(w, "forgot_password.html", map[string]interface{}{
		"ActiveNav": "login",
		"Success":   "Si existe una cuenta asociada a este correo, recibirás un enlace para restablecer tu contraseña en breve.",
	})
}

func (h *AuthHandler) ShowResetPassword(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Redirect(w, r, "/forgot-password", http.StatusSeeOther)
		return
	}

	_, err := h.repo.GetUserByResetToken(token)
	if err != nil {
		h.renderer.RenderPage(w, "reset_password.html", map[string]interface{}{
			"ActiveNav": "login",
			"Error":     "El enlace de restablecimiento es inválido o ha expirado. Por favor solicita uno nuevo.",
			"Invalid":   true,
		})
		return
	}

	h.renderer.RenderPage(w, "reset_password.html", map[string]interface{}{
		"ActiveNav": "login",
		"Token":     token,
	})
}

func (h *AuthHandler) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	token := strings.TrimSpace(r.FormValue("token"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if len(password) < 6 {
		h.renderer.RenderPage(w, "reset_password.html", map[string]interface{}{
			"ActiveNav": "login",
			"Token":     token,
			"Error":     "La contraseña debe tener al menos 6 caracteres.",
		})
		return
	}

	if password != confirmPassword {
		h.renderer.RenderPage(w, "reset_password.html", map[string]interface{}{
			"ActiveNav": "login",
			"Token":     token,
			"Error":     "Las contraseñas no coinciden.",
		})
		return
	}

	newHash, err := h.authService.HashPassword(password)
	if err != nil {
		http.Error(w, "Error cifrando contraseña", http.StatusInternalServerError)
		return
	}

	if err := h.repo.ResetPasswordWithToken(token, newHash); err != nil {
		h.renderer.RenderPage(w, "reset_password.html", map[string]interface{}{
			"ActiveNav": "login",
			"Token":     token,
			"Error":     err.Error(),
		})
		return
	}

	http.Redirect(w, r, "/login?reset_success=1", http.StatusSeeOther)
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	h.authService.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

