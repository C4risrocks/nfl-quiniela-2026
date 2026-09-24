package notifications

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"strings"
	"time"

	"nfl-quiniela-2026/db"
)

type EmailSender struct {
	host       string
	port       int
	username   string
	password   string
	from       string
	appBaseURL string
}

func NewEmailSender(host string, port int, username, password, from, appBaseURL string) *EmailSender {
	return &EmailSender{
		host:       host,
		port:       port,
		username:   strings.TrimSpace(username),
		password:   strings.ReplaceAll(strings.TrimSpace(password), " ", ""),
		from:       strings.TrimSpace(from),
		appBaseURL: appBaseURL,
	}
}

func (s *EmailSender) BaseURL() string {
	return s.appBaseURL
}

const emailTemplateHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Recordatorio NFL Quiniela 2026</title>
<style>
  body { margin: 0; padding: 0; background-color: #000000; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; color: #ededed; }
  .container { max-width: 580px; margin: 30px auto; background-color: #0c0c0c; border: 1px solid #222222; border-radius: 16px; padding: 32px; }
  .badge { display: inline-block; background-color: rgba(234, 179, 8, 0.1); color: #facc15; border: 1px solid rgba(234, 179, 8, 0.3); border-radius: 9999px; font-size: 11px; font-weight: 600; padding: 4px 12px; margin-bottom: 16px; text-transform: uppercase; letter-spacing: 0.05em; }
  h1 { font-size: 22px; font-weight: 800; color: #ffffff; margin: 0 0 12px 0; letter-spacing: -0.02em; }
  p { font-size: 14px; line-height: 1.6; color: #a1a1aa; margin: 0 0 20px 0; }
  .alert-box { background-color: #141414; border: 1px solid #27272a; border-radius: 12px; padding: 18px; margin-bottom: 24px; }
  .alert-box strong { color: #ffffff; }
  .btn { display: inline-block; background-color: #ffffff; color: #000000; font-size: 13px; font-weight: 700; text-decoration: none; padding: 12px 24px; border-radius: 10px; text-align: center; }
  .btn:hover { background-color: #e4e4e7; }
  .footer { margin-top: 32px; padding-top: 20px; border-top: 1px solid #1f1f1f; font-size: 11px; color: #71717a; text-align: center; }
</style>
</head>
<body>
<div class="container">
  <div class="badge">⏰ Recordatorio de Quiniela</div>
  <h1>¡No te quedes sin puntos, {{.Username}}!</h1>
  <p>El primer partido de <strong>{{.WeekName}}</strong> está por comenzar y detectamos que aún tienes pronósticos pendientes de registrar.</p>
  
  <div class="alert-box">
    <div style="font-size: 12px; color: #71717a; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 4px;">Hora estimada de patada inicial:</div>
    <div style="font-size: 16px; font-weight: 700; color: #ffffff;">{{.FormattedKickoff}}</div>
    <div style="font-size: 12px; color: #eab308; margin-top: 6px;">Recuerda: Los partidos se bloquean automáticamente en cuanto inician.</div>
  </div>

  <div style="text-align: center; margin: 28px 0;">
    <a href="{{.PicksURL}}" class="btn">Ingresar mis Pronósticos &rarr;</a>
  </div>

  <div class="footer">
    NFL Quiniela 2026 &bull; Este es un correo automático para participantes activos.
  </div>
</div>
</body>
</html>`

const gracePeriodTemplateHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Aviso de Prórroga NFL Quiniela 2026</title>
<style>
  body { margin: 0; padding: 0; background-color: #000000; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; color: #ededed; }
  .container { max-width: 580px; margin: 30px auto; background-color: #0c0c0c; border: 1px solid #222222; border-radius: 16px; padding: 32px; }
  .badge { display: inline-block; background-color: rgba(59, 130, 246, 0.15); color: #60a5fa; border: 1px solid rgba(59, 130, 246, 0.3); border-radius: 9999px; font-size: 11px; font-weight: 600; padding: 4px 12px; margin-bottom: 16px; text-transform: uppercase; letter-spacing: 0.05em; }
  h1 { font-size: 22px; font-weight: 800; color: #ffffff; margin: 0 0 12px 0; letter-spacing: -0.02em; }
  p { font-size: 14px; line-height: 1.6; color: #a1a1aa; margin: 0 0 20px 0; }
  .alert-box { background-color: #111827; border: 1px solid #1e3a8a; border-radius: 12px; padding: 18px; margin-bottom: 24px; }
  .alert-box strong { color: #ffffff; }
  .btn { display: inline-block; background-color: #ffffff; color: #000000; font-size: 13px; font-weight: 700; text-decoration: none; padding: 12px 24px; border-radius: 10px; text-align: center; }
  .btn:hover { background-color: #e4e4e7; }
  .footer { margin-top: 32px; padding-top: 20px; border-top: 1px solid #1f1f1f; font-size: 11px; color: #71717a; text-align: center; }
</style>
</head>
<body>
<div class="container">
  <div class="badge">⏳ Prórroga Activa &bull; Semana 1</div>
  <h1>¡Últimas horas, {{.Username}}!</h1>
  <p>La prórroga especial para la <strong>Semana 1</strong> está por concluir. Debido al despliegue de la quiniela, <strong>todos los partidos de la jornada</strong> (incluyendo el partido de Seattle vs New England) se encuentran aún abiertos para registro y edición.</p>
  
  <div class="alert-box">
    <div style="font-size: 12px; color: #93c5fd; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 4px;">Cierre definitivo de pronósticos:</div>
    <div style="font-size: 16px; font-weight: 700; color: #ffffff;">{{.FormattedDeadline}}</div>
    <div style="font-size: 12px; color: #60a5fa; margin-top: 6px;">Al llegar la hora límite, la jornada se bloqueará por completo y no podrás ingresar selecciones.</div>
  </div>

  <div style="text-align: center; margin: 28px 0;">
    <a href="{{.PicksURL}}" class="btn">Completar mis Pronósticos Ahora &rarr;</a>
  </div>

  <div class="footer">
    NFL Quiniela 2026 &bull; Notificación de prórroga especial para participantes con pronósticos pendientes.
  </div>
</div>
</body>
</html>`

const verificationTemplateHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Verifica tu Correo - NFL Quiniela 2026</title>
<style>
  body { margin: 0; padding: 0; background-color: #000000; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; color: #ededed; }
  .container { max-width: 580px; margin: 30px auto; background-color: #0c0c0c; border: 1px solid #222222; border-radius: 16px; padding: 32px; }
  .badge { display: inline-block; background-color: rgba(16, 185, 129, 0.1); color: #34d399; border: 1px solid rgba(16, 185, 129, 0.3); border-radius: 9999px; font-size: 11px; font-weight: 600; padding: 4px 12px; margin-bottom: 16px; text-transform: uppercase; letter-spacing: 0.05em; }
  h1 { font-size: 22px; font-weight: 800; color: #ffffff; margin: 0 0 12px 0; letter-spacing: -0.02em; }
  p { font-size: 14px; line-height: 1.6; color: #a1a1aa; margin: 0 0 20px 0; }
  .btn { display: inline-block; background-color: #ffffff; color: #000000; font-size: 13px; font-weight: 700; text-decoration: none; padding: 12px 28px; border-radius: 10px; text-align: center; }
  .btn:hover { background-color: #e4e4e7; }
  .url-box { background-color: #141414; border: 1px solid #27272a; border-radius: 8px; padding: 12px; font-family: monospace; font-size: 11px; color: #a1a1aa; word-break: break-all; margin-top: 20px; }
  .footer { margin-top: 32px; padding-top: 20px; border-top: 1px solid #1f1f1f; font-size: 11px; color: #71717a; text-align: center; }
</style>
</head>
<body>
<div class="container">
  <div class="badge">🏈 NFL Quiniela 2026</div>
  <h1>¡Bienvenido, {{.Username}}!</h1>
  <p>Gracias por unirte a la temporada 2026 de la NFL Quiniela. Para confirmar tu cuenta y habilitar las alertas y recordatorios de jornada, haz clic en el siguiente botón:</p>
  
  <div style="text-align: center; margin: 28px 0;">
    <a href="{{.VerifyURL}}" class="btn">Confirmar mi Cuenta &rarr;</a>
  </div>

  <p style="font-size: 12px; color: #71717a;">Si el botón no funciona, copia y pega este enlace en tu navegador:</p>
  <div class="url-box">{{.VerifyURL}}</div>

  <div class="footer">
    NFL Quiniela 2026 &bull; Si tú no creaste esta cuenta, puedes ignorar este mensaje.
  </div>
</div>
</body>
</html>`

const passwordResetTemplateHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Restablecer Contraseña - NFL Quiniela 2026</title>
<style>
  body { margin: 0; padding: 0; background-color: #000000; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; color: #ededed; }
  .container { max-width: 580px; margin: 30px auto; background-color: #0c0c0c; border: 1px solid #222222; border-radius: 16px; padding: 32px; }
  .badge { display: inline-block; background-color: rgba(239, 68, 68, 0.1); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.3); border-radius: 9999px; font-size: 11px; font-weight: 600; padding: 4px 12px; margin-bottom: 16px; text-transform: uppercase; letter-spacing: 0.05em; }
  h1 { font-size: 22px; font-weight: 800; color: #ffffff; margin: 0 0 12px 0; letter-spacing: -0.02em; }
  p { font-size: 14px; line-height: 1.6; color: #a1a1aa; margin: 0 0 20px 0; }
  .btn { display: inline-block; background-color: #ffffff; color: #000000; font-size: 13px; font-weight: 700; text-decoration: none; padding: 12px 28px; border-radius: 10px; text-align: center; }
  .btn:hover { background-color: #e4e4e7; }
  .alert-warn { background-color: #17120e; border: 1px solid #451a03; border-radius: 8px; padding: 12px; font-size: 12px; color: #f97316; margin: 16px 0; }
  .url-box { background-color: #141414; border: 1px solid #27272a; border-radius: 8px; padding: 12px; font-family: monospace; font-size: 11px; color: #a1a1aa; word-break: break-all; margin-top: 20px; }
  .footer { margin-top: 32px; padding-top: 20px; border-top: 1px solid #1f1f1f; font-size: 11px; color: #71717a; text-align: center; }
</style>
</head>
<body>
<div class="container">
  <div class="badge">🔒 Seguridad</div>
  <h1>Restablecer Contraseña</h1>
  <p>Hola {{.Username}}, recibimos una solicitud para cambiar la contraseña de tu cuenta en NFL Quiniela 2026.</p>
  
  <div style="text-align: center; margin: 28px 0;">
    <a href="{{.ResetURL}}" class="btn">Restablecer mi Contraseña &rarr;</a>
  </div>

  <div class="alert-warn">
    Este enlace tiene una validez de <strong>60 minutos</strong> por seguridad.
  </div>

  <p style="font-size: 12px; color: #71717a;">Si no solicitaste este cambio, simplemente ignora este correo; tu contraseña no cambiará.</p>
  <div class="url-box">{{.ResetURL}}</div>

  <div class="footer">
    NFL Quiniela 2026 &bull; Soporte de seguridad.
  </div>
</div>
</body>
</html>`

const weeklyRecapTemplateHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Resumen Semanal NFL Quiniela 2026</title>
<style>
  body { margin: 0; padding: 0; background-color: #000000; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; color: #ededed; }
  .container { max-width: 600px; margin: 30px auto; background-color: #0c0c0c; border: 1px solid #222222; border-radius: 16px; padding: 32px; }
  .badge { display: inline-block; background-color: rgba(234, 179, 8, 0.12); color: #facc15; border: 1px solid rgba(234, 179, 8, 0.3); border-radius: 9999px; font-size: 11px; font-weight: 700; padding: 4px 12px; margin-bottom: 16px; text-transform: uppercase; letter-spacing: 0.05em; }
  h1 { font-size: 24px; font-weight: 900; color: #ffffff; margin: 0 0 8px 0; letter-spacing: -0.02em; }
  p { font-size: 14px; line-height: 1.6; color: #a1a1aa; margin: 0 0 20px 0; }
  .card { background-color: #141414; border: 1px solid #27272a; border-radius: 12px; padding: 20px; margin-bottom: 24px; }
  .btn-row { text-align: center; margin: 28px 0 10px 0; }
  .btn-primary { display: inline-block; background-color: #ffffff; color: #000000; font-size: 13px; font-weight: 700; text-decoration: none; padding: 12px 24px; border-radius: 10px; margin: 4px; }
  .btn-secondary { display: inline-block; background-color: #27272a; color: #ffffff; font-size: 13px; font-weight: 600; text-decoration: none; padding: 12px 20px; border-radius: 10px; margin: 4px; }
  .footer { margin-top: 32px; padding-top: 20px; border-top: 1px solid #1f1f1f; font-size: 11px; color: #71717a; text-align: center; }
</style>
</head>
<body>
<div class="container">
  <div class="badge">🏆 Resultados Oficiales &bull; {{.Recap.WeekName}}</div>
  <h1>¡La jornada ha concluido!</h1>
  <p>Hola {{.Username}}, todos los partidos de <strong>{{.Recap.WeekName}}</strong> han finalizado y los puntajes han sido calculados.</p>

  {{if .Recap.UserRecap}}
  <div class="card" style="border-left: 4px solid #3b82f6;">
    <div style="font-size: 12px; font-weight: 700; color: #60a5fa; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 8px;">
      Tu Desempeño Personal
    </div>
    <div style="font-size: 15px; color: #e4e4e7; margin-bottom: 12px;">
      Terminaste en la posición <strong>#{{.Recap.UserRecap.WeeklyRank}}</strong> de {{.Recap.TotalParticipants}} participantes en esta jornada.
    </div>

    <table width="100%" cellpadding="0" cellspacing="6" style="margin-top: 10px;">
      <tr>
        <td align="center" style="background-color: #1a1a1a; border: 1px solid #2e2e33; border-radius: 8px; padding: 10px; width: 25%;">
          <div style="font-size: 18px; font-weight: 800; color: #facc15; font-family: monospace;">{{.Recap.UserRecap.TotalPoints}}</div>
          <div style="font-size: 9px; color: #a1a1aa; text-transform: uppercase; margin-top: 2px;">Puntos</div>
        </td>
        <td align="center" style="background-color: #1a1a1a; border: 1px solid #2e2e33; border-radius: 8px; padding: 10px; width: 25%;">
          <div style="font-size: 18px; font-weight: 800; color: #34d399; font-family: monospace;">{{.Recap.UserRecap.CorrectPicks}}/{{.Recap.UserRecap.TotalGames}}</div>
          <div style="font-size: 9px; color: #a1a1aa; text-transform: uppercase; margin-top: 2px;">Aciertos</div>
        </td>
        <td align="center" style="background-color: #1a1a1a; border: 1px solid #2e2e33; border-radius: 8px; padding: 10px; width: 25%;">
          <div style="font-size: 18px; font-weight: 800; color: #60a5fa; font-family: monospace;">{{.Recap.UserRecap.AccuracyPercent}}%</div>
          <div style="font-size: 9px; color: #a1a1aa; text-transform: uppercase; margin-top: 2px;">Efectividad</div>
        </td>
        <td align="center" style="background-color: #1a1a1a; border: 1px solid #2e2e33; border-radius: 8px; padding: 10px; width: 25%;">
          <div style="font-size: 18px; font-weight: 800; color: #ffffff; font-family: monospace;">#{{.Recap.UserRecap.SeasonRank}}</div>
          <div style="font-size: 9px; color: #a1a1aa; text-transform: uppercase; margin-top: 2px;">General ({{.Recap.UserRecap.SeasonTotalPts}} pts)</div>
        </td>
      </tr>
    </table>
  </div>
  {{end}}

  <!-- Podium Section -->
  <div class="card">
    <div style="font-size: 12px; font-weight: 700; color: #facc15; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 12px;">
      Podio de la Jornada
    </div>
    <table width="100%" cellpadding="6" cellspacing="0" style="font-size: 13px;">
      {{range .Recap.Podium}}
      <tr style="border-bottom: 1px solid #222222;">
        <td width="30" align="center" style="font-weight: 800; font-size: 15px;">
          {{if eq .Rank 1}}🥇{{else if eq .Rank 2}}🥈{{else if eq .Rank 3}}🥉{{else}}#{{.Rank}}{{end}}
        </td>
        <td style="font-weight: 700; color: #ffffff;">
          {{.Username}}
          {{if .FavoriteTeamCode}}<span style="font-size: 10px; background: #27272a; padding: 2px 6px; border-radius: 4px; color: #a1a1aa; margin-left: 6px;">{{.FavoriteTeamCode}}</span>{{end}}
        </td>
        <td align="right" style="font-family: monospace; font-weight: 700; color: #facc15;">
          {{.TotalPoints}} pts
        </td>
        <td align="right" style="font-family: monospace; font-size: 11px; color: #a1a1aa;">
          ({{.CorrectPicks}}/{{.TotalPicks}} aciertos)
        </td>
      </tr>
      {{end}}
    </table>
  </div>

  <div class="btn-row">
    <a href="{{.LeaderboardURL}}" class="btn-primary">Ver Tabla de Posiciones Completa &rarr;</a>
    {{if .NextWeekPicksURL}}
    <a href="{{.NextWeekPicksURL}}" class="btn-secondary">Pronósticos {{.Recap.NextWeekName}} &rarr;</a>
    {{end}}
  </div>

  <div class="footer">
    NFL Quiniela 2026 &bull; Puedes modificar tus preferencias de notificación en tu <a href="{{.ProfileURL}}" style="color: #a1a1aa; text-decoration: underline;">Perfil</a>.
  </div>
</div>
</body>
</html>`


type EmailTemplateData struct {
	Username         string
	WeekName         string
	FormattedKickoff string
	PicksURL         string
}

// SendKickoffReminder sends an email alert to a user with pending picks
func (s *EmailSender) SendKickoffReminder(user *db.User, week *db.Week, kickoff time.Time) error {
	subject := fmt.Sprintf("🏈 ¡Recordatorio NFL! Completa tus pronósticos de %s", week.Name)
	picksURL := fmt.Sprintf("%s/picks?week=%d", s.appBaseURL, week.WeekNumber)

	data := EmailTemplateData{
		Username:         user.Username,
		WeekName:         week.Name,
		FormattedKickoff: kickoff.Format("Monday 02 Jan, 03:04 PM MST"),
		PicksURL:         picksURL,
	}

	tmpl, err := template.New("email").Parse(emailTemplateHTML)
	if err != nil {
		return fmt.Errorf("parsing email template: %w", err)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("executing email template: %w", err)
	}

	return s.sendMail(user.Email, user.Username, subject, body.String())
}

// SendGracePeriodReminder sends an urgent reminder about the closing of the Week 1 grace period
func (s *EmailSender) SendGracePeriodReminder(user *db.User, week *db.Week, deadline time.Time) error {
	subject := fmt.Sprintf("⏳ ¡Últimas horas de prórroga! Completa tus pronósticos de %s", week.Name)
	picksURL := fmt.Sprintf("%s/picks?week=%d", s.appBaseURL, week.WeekNumber)

	formattedDeadline := deadline.Format("Monday 02 Jan, 03:04 PM MST")
	loc, err := time.LoadLocation("America/Mexico_City")
	if err == nil {
		cdmxTime := deadline.In(loc)
		dayAbbr := map[string]string{"Mon": "Lunes", "Tue": "Martes", "Wed": "Miércoles", "Thu": "Jueves", "Fri": "Viernes", "Sat": "Sábado", "Sun": "Domingo"}[cdmxTime.Format("Mon")]
		monthAbbr := map[string]string{"Jan": "Enero", "Feb": "Febrero", "Mar": "Marzo", "Apr": "Abril", "May": "Mayo", "Jun": "Junio", "Jul": "Julio", "Aug": "Agosto", "Sep": "Septiembre", "Oct": "Octubre", "Nov": "Noviembre", "Dec": "Diciembre"}[cdmxTime.Format("Jan")]
		formattedDeadline = fmt.Sprintf("%s %s de %s a las %s (Hora CDMX)", dayAbbr, cdmxTime.Format("2"), monthAbbr, cdmxTime.Format("3:04 PM"))
	}

	data := struct {
		Username          string
		WeekName          string
		FormattedDeadline string
		PicksURL          string
	}{
		Username:          user.Username,
		WeekName:          week.Name,
		FormattedDeadline: formattedDeadline,
		PicksURL:          picksURL,
	}

	tmpl, err := template.New("grace_email").Parse(gracePeriodTemplateHTML)
	if err != nil {
		return fmt.Errorf("parsing grace email template: %w", err)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("executing grace email template: %w", err)
	}

	return s.sendMail(user.Email, user.Username, subject, body.String())
}

// SendVerificationEmail sends an email confirmation link upon signup or resend request
func (s *EmailSender) SendVerificationEmail(user *db.User, token string, customBaseURL ...string) error {
	subject := "🏈 Confirma tu correo para NFL Quiniela 2026"
	baseURL := s.appBaseURL
	if len(customBaseURL) > 0 && customBaseURL[0] != "" {
		baseURL = customBaseURL[0]
	}
	baseURL = strings.TrimRight(baseURL, "/")
	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", baseURL, token)

	data := struct {
		Username  string
		VerifyURL string
	}{
		Username:  user.Username,
		VerifyURL: verifyURL,
	}

	tmpl, err := template.New("verify").Parse(verificationTemplateHTML)
	if err != nil {
		return fmt.Errorf("parsing verification template: %w", err)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("executing verification template: %w", err)
	}

	return s.sendMail(user.Email, user.Username, subject, body.String())
}

// SendPasswordResetEmail sends a secure password reset link valid for 60 minutes
func (s *EmailSender) SendPasswordResetEmail(user *db.User, token string, customBaseURL ...string) error {
	subject := "🔒 Restablece tu contraseña - NFL Quiniela 2026"
	baseURL := s.appBaseURL
	if len(customBaseURL) > 0 && customBaseURL[0] != "" {
		baseURL = customBaseURL[0]
	}
	baseURL = strings.TrimRight(baseURL, "/")
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)

	data := struct {
		Username string
		ResetURL string
	}{
		Username: user.Username,
		ResetURL: resetURL,
	}

	tmpl, err := template.New("reset").Parse(passwordResetTemplateHTML)
	if err != nil {
		return fmt.Errorf("parsing reset template: %w", err)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("executing reset template: %w", err)
	}

	return s.sendMail(user.Email, user.Username, subject, body.String())
}

// SendWeeklyRecapEmail sends a personalized weekly recap email with podium and individual stats
func (s *EmailSender) SendWeeklyRecapEmail(user *db.User, recap *db.WeeklyRecapData, customBaseURL ...string) error {
	subject := fmt.Sprintf("🏆 Resumen Oficial: %s - NFL Quiniela 2026", recap.WeekName)
	baseURL := s.appBaseURL
	if len(customBaseURL) > 0 && customBaseURL[0] != "" {
		baseURL = customBaseURL[0]
	}
	baseURL = strings.TrimRight(baseURL, "/")

	leaderboardURL := fmt.Sprintf("%s/leaderboard?week=%d", baseURL, recap.WeekNumber)
	profileURL := fmt.Sprintf("%s/profile", baseURL)
	nextWeekPicksURL := ""
	if recap.NextWeekNumber > 0 {
		nextWeekPicksURL = fmt.Sprintf("%s/picks?week=%d", baseURL, recap.NextWeekNumber)
	}

	data := struct {
		Username         string
		Recap            *db.WeeklyRecapData
		LeaderboardURL   string
		NextWeekPicksURL string
		ProfileURL       string
	}{
		Username:         user.Username,
		Recap:            recap,
		LeaderboardURL:   leaderboardURL,
		NextWeekPicksURL: nextWeekPicksURL,
		ProfileURL:       profileURL,
	}

	tmpl, err := template.New("recap").Parse(weeklyRecapTemplateHTML)
	if err != nil {
		return fmt.Errorf("parsing recap template: %w", err)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("executing recap template: %w", err)
	}

	return s.sendMail(user.Email, user.Username, subject, body.String())
}


// sendMail handles SMTP dispatch via Port 587 (STARTTLS) or Port 465 (SSL) or Mock mode
func (s *EmailSender) sendMail(toEmail, toName, subject, htmlBody string) error {
	// If no SMTP host configured, run in log/preview mode
	if s.host == "" {
		log.Printf("[Email - MOCK] To: %s <%s> | Subject: %s", toName, toEmail, subject)
		return nil
	}

	// Prepare MIME message
	headers := make(map[string]string)
	headers["From"] = s.from
	headers["To"] = toEmail
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	// Plain auth if credentials provided
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	// Connect and send
	if s.port == 465 {
		// SSL connection
		tlsConfig := &tls.Config{ServerName: s.host}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Quit()

		if auth != nil {
			if err := client.Auth(tlsAuthWrapper{auth}); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
		if err := client.Mail(s.from); err != nil {
			return err
		}
		if err := client.Rcpt(toEmail); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(msg.String())); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		log.Printf("[Email] Sent email successfully to %s <%s> (Port 465 SSL).", toName, toEmail)
		return nil
	}

	// Standard STARTTLS / port 587 or 25
	err := smtp.SendMail(addr, auth, s.from, []string{toEmail}, []byte(msg.String()))
	if err != nil {
		return fmt.Errorf("smtp sendmail: %w", err)
	}

	log.Printf("[Email] Sent email successfully to %s <%s> (Port 587 STARTTLS).", toName, toEmail)
	return nil
}

// tlsAuthWrapper wraps smtp.Auth to force the TLS flag to true when authenticating over implicit TLS (port 465)
type tlsAuthWrapper struct {
	smtp.Auth
}

func (a tlsAuthWrapper) Start(server *smtp.ServerInfo) (string, []byte, error) {
	s := *server
	s.TLS = true
	return a.Auth.Start(&s)
}
