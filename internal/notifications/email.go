package notifications

import (
	"context"
	"fmt"
	"net/smtp"
	"strconv"
	"strings"
)

type EmailAdapter struct{}

func (e *EmailAdapter) Type() string { return ChannelTypeEmail }

func (e *EmailAdapter) Send(_ context.Context, config map[string]any, payload map[string]any) error {
	host, _ := config["smtp_host"].(string)
	if host == "" {
		return fmt.Errorf("email config missing smtp_host")
	}

	port := 587
	if p, ok := config["smtp_port"]; ok {
		switch v := p.(type) {
		case float64:
			port = int(v)
		case string:
			if parsed, err := strconv.Atoi(v); err == nil {
				port = parsed
			}
		}
	}

	user, _ := config["smtp_user"].(string)
	pass, _ := config["smtp_pass"].(string)
	from, _ := config["from"].(string)
	if from == "" {
		from = user
	}

	to, _ := payload["to"].(string)
	if to == "" {
		return fmt.Errorf("email payload missing to")
	}

	subject, _ := payload["subject"].(string)
	body, _ := payload["body"].(string)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", host, port)

	var auth smtp.Auth
	if user != "" && pass != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	recipients := strings.Split(to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	if err := smtp.SendMail(addr, auth, from, recipients, []byte(msg)); err != nil {
		return fmt.Errorf("email send failed: %w", err)
	}

	return nil
}
