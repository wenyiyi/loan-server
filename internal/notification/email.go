package notification

import (
	applog "LoanService/pkg/log"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"LoanService/config"
)

type Sender interface {
	Send(context.Context, string, string, string) error
}
type SMTP struct {
	Address, From, Username, Password string
	AllowPlaintext                    bool
}

func NewSMTP(conf config.SMTPConfig) SMTP {
	return SMTP{Address: conf.Address, From: conf.From, Username: conf.Username, Password: conf.Password, AllowPlaintext: conf.AllowPlaintext}
}

func (s SMTP) Send(ctx context.Context, to, link, messageID string) error {
	if _, err := mail.ParseAddress(to); err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	if _, err := mail.ParseAddress(s.From); err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	if strings.ContainsAny(to+s.From+link+messageID, "\r\n") {
		return fmt.Errorf("invalid email header")
	}
	host, _, err := net.SplitHostPort(s.Address)
	if err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", s.Address)
	if err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			applog.Error(ctx, "email.Send failed", err)
			return err
		}
	} else if !s.AllowPlaintext {
		return fmt.Errorf("SMTP requires STARTTLS")
	}
	if s.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			applog.Error(ctx, "email.Send failed", err)
			return err
		}
	}
	if err = client.Mail(s.From); err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	if err = client.Rcpt(to); err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	w, err := client.Data()
	if err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	_, err = fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: Loan fully funded\r\nMessage-ID: <%s@loan-service>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYour loan is fully funded. Agreement PDF: %s\r\n", s.From, to, messageID, link)
	if err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	if err = w.Close(); err != nil {
		applog.Error(ctx, "email.Send failed", err)
		return err
	}
	return client.Quit()
}
