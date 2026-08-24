// Package mailer envía emails transaccionales (§2.2 magic link) vía SMTP.
// Si no hay SMTP configurado, cae a modo "log": escribe el cuerpo en el log
// del servidor en vez de perder el link. En dev eso es suficiente para probar
// el flujo; en producción se espera MAIL_HOST/MAIL_PORT/MAIL_FROM.
package mailer

import (
	"fmt"
	"log"
	"strings"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"gopkg.in/gomail.v2"
)

// Sender abstrae el envío de un email transaccional.
type Sender interface {
	Send(to, subject, htmlBody string) error
}

// SMTP envía por SMTP real (gomail).
type SMTP struct {
	cfg       *config.Config
	fromName  string
	fromAddr  string
	replyTo   string
}

// LogSender es el fallback de desarrollo: vuelca el email al log.
type LogSender struct{}

// New construye el sender correcto según la config.
func New(cfg *config.Config) Sender {
	if cfg.MailConfigured() {
		return &SMTP{
			cfg:      cfg,
			fromName: cfg.MailFromName,
			fromAddr: cfg.MailFrom,
			replyTo:  cfg.MailFrom,
		}
	}
	log.Printf("[mailer] SMTP no configurado: usando fallback a log (MAIL_HOST/MAIL_PORT/MAIL_FROM)")
	return &LogSender{}
}

func (s *SMTP) Send(to, subject, htmlBody string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", m.FormatAddress(s.fromAddr, s.fromName))
	m.SetHeader("To", to)
	m.SetHeader("Reply-To", s.replyTo)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)
	m.AddAlternative("text/plain", htmlToText(htmlBody))

	d := gomail.NewDialer(s.cfg.MailHost, s.cfg.MailPort, s.cfg.MailUser, s.cfg.MailPassword)
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("mailer: enviar a %s: %w", to, err)
	}
	return nil
}

func (s *LogSender) Send(to, subject, htmlBody string) error {
	log.Printf("[magic-link] para %s — asunto %q\n%s", to, subject, htmlBody)
	return nil
}

// htmlToText es un downgrade best-effort para clientes sin HTML.
func htmlToText(html string) string {
	repl := []string{
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</p>", "\n",
		"<p>", "",
		"<strong>", "",
		"</strong>", "",
		"<a href=\"", "",
		"\">", ": ",
		"</a>", "",
		"<em>", "",
		"</em>", "",
	}
	for i := 0; i+2 <= len(repl); i += 2 {
		html = strings.ReplaceAll(html, repl[i], repl[i+1])
	}
	return html
}
