package notifications

import (
	"context"
	"fmt"
	"math"
	"net/smtp"
	"strconv"
	"strings"
)

const defaultSMTPPort = 587

type EmailAdapter struct{}

func (e *EmailAdapter) Type() string { return ChannelTypeEmail }

func (e *EmailAdapter) Send(_ context.Context, config map[string]any, payload map[string]any) error {
	host, _ := config["smtp_host"].(string)
	if host == "" {
		return fmt.Errorf("email config missing smtp_host")
	}

	port, err := smtpPortFromConfig(config["smtp_port"])
	if err != nil {
		return err
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

func smtpPortFromConfig(raw any) (int, error) {
	if raw == nil {
		return defaultSMTPPort, nil
	}

	var port int64
	switch typed := raw.(type) {
	case int:
		port = int64(typed)
	case int64:
		port = typed
	case float64:
		if math.IsInf(typed, 0) || math.IsNaN(typed) || math.Trunc(typed) != typed || typed < 1 || typed > 65535 {
			return 0, fmt.Errorf("email config invalid smtp_port")
		}
		port = int64(typed)
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return defaultSMTPPort, nil
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("email config invalid smtp_port")
		}
		port = parsed
	default:
		return 0, fmt.Errorf("email config invalid smtp_port")
	}

	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("email config smtp_port out of range")
	}
	return int(port), nil
}
