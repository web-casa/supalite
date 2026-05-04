package handler

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/supalite/admin/internal/envfile"
)

type smtpTestRequest struct {
	To string `json:"to"`
}

// HandleSMTPTest sends a test message using the SMTP settings currently
// saved in .env. Reading from .env (not os.Getenv) means users can Save
// new settings and immediately Test, without restarting GoTrue.
//
// Port policy:
//   - 465  → implicit TLS (SMTPS)
//   - 587  → STARTTLS (required)
//   - 25   → plaintext, then opportunistic STARTTLS if offered
//
// Auth: PLAIN if user+pass both present, otherwise unauthenticated
// (some internal relays accept that).
func (d *Deps) HandleSMTPTest(w http.ResponseWriter, r *http.Request) {
	var req smtpTestRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// ParseAddress accepts display-name forms like "Alice <a@b.com>". We
	// keep only the .Address for the SMTP envelope; otherwise client.Rcpt
	// builds a malformed `RCPT TO:<Alice <a@b.com>>`.
	toAddr, err := mail.ParseAddress(req.To)
	if err != nil {
		writeError(w, 400, "invalid recipient address")
		return
	}

	env, err := envfile.Read(d.EnvFile)
	if err != nil {
		writeError(w, 500, "read env: "+err.Error())
		return
	}
	host := strings.TrimSpace(env["GOTRUE_SMTP_HOST"])
	portStr := strings.TrimSpace(env["GOTRUE_SMTP_PORT"])
	user := env["GOTRUE_SMTP_USER"]
	pass := env["GOTRUE_SMTP_PASS"]
	fromRaw := strings.TrimSpace(env["GOTRUE_SMTP_ADMIN_EMAIL"])

	if host == "" {
		writeError(w, 400, "GOTRUE_SMTP_HOST not set")
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		writeError(w, 400, "invalid SMTP port")
		return
	}
	if fromRaw == "" {
		writeError(w, 400, "GOTRUE_SMTP_ADMIN_EMAIL (sender) not set")
		return
	}
	fromAddr, err := mail.ParseAddress(fromRaw)
	if err != nil {
		writeError(w, 400, "invalid sender address: "+err.Error())
		return
	}

	// One absolute deadline for the whole operation (dial + TLS + SMTP).
	// Previously dial (10s) + conversation (15s) could exceed the 15s
	// handler ctx. Single deadline keeps all three paths bounded.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := sendSMTPTest(ctx, smtpParams{
		Host: host, Port: port, User: user, Pass: pass,
		FromEnvelope: fromAddr.Address,
		ToEnvelope:   toAddr.Address,
		FromHeader:   fromAddr.String(),
		ToHeader:     toAddr.String(),
	}); err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"sent": true, "to": toAddr.Address})
}

type smtpParams struct {
	Host         string
	Port         int
	User, Pass   string
	// Envelope addresses (bare) — used for MAIL FROM / RCPT TO.
	FromEnvelope, ToEnvelope string
	// Header strings (may include display name) — used in message headers.
	FromHeader, ToHeader string
}

func sendSMTPTest(ctx context.Context, p smtpParams) error {
	addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(15 * time.Second)
	}
	netDialer := &net.Dialer{}
	tlsCfg := &tls.Config{ServerName: p.Host}

	var conn net.Conn
	var err error
	switch p.Port {
	case 465:
		// Implicit TLS. tls.Dialer honors the parent ctx for the TCP
		// dial + TLS handshake; plain tls.Dial would ignore it.
		tlsDialer := &tls.Dialer{NetDialer: netDialer, Config: tlsCfg}
		conn, err = tlsDialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
	default:
		conn, err = netDialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("dial: %w", err)
		}
	}
	// Propagate the ctx deadline to per-read/write deadlines on the conn.
	_ = conn.SetDeadline(deadline)

	client, err := smtp.NewClient(conn, p.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	// For plaintext ports, upgrade via STARTTLS if the server advertises it.
	// Port 587 treats STARTTLS as mandatory; 25 as opportunistic.
	if p.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		} else if p.Port == 587 {
			return fmt.Errorf("server does not advertise STARTTLS on port 587")
		}
	}

	if p.User != "" && p.Pass != "" {
		auth := smtp.PlainAuth("", p.User, p.Pass, p.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}

	if err := client.Mail(p.FromEnvelope); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := client.Rcpt(p.ToEnvelope); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	msg := buildTestMessage(p.FromHeader, p.ToHeader)
	if _, err := wc.Write([]byte(msg)); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	return client.Quit()
}

func buildTestMessage(from, to string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: SupaLite SMTP test\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString("If you received this, your SMTP settings work.\r\n")
	b.WriteString("Sent from the SupaLite admin panel.\r\n")
	return b.String()
}
