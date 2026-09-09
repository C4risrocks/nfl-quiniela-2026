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
		username:   username,
		password:   password,
		from:       from,
		appBaseURL: appBaseURL,
	}
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

	// If no SMTP host configured, run in log/preview mode
	if s.host == "" {
		log.Printf("[Email - MOCK] To: %s <%s> | Subject: %s | URL: %s", user.Username, user.Email, subject, picksURL)
		return nil
	}

	// Prepare MIME message
	headers := make(map[string]string)
	headers["From"] = s.from
	headers["To"] = user.Email
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.Write(body.Bytes())

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
		if err := client.Rcpt(user.Email); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(msg.String())); err != nil {
			return err
		}
		return w.Close()
	}

	// Standard STARTTLS / port 587 or 25
	err = smtp.SendMail(addr, auth, s.from, []string{user.Email}, []byte(msg.String()))
	if err != nil {
		return fmt.Errorf("smtp sendmail: %w", err)
	}

	log.Printf("[Email] Sent reminder successfully to %s (%s).", user.Username, user.Email)
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
