package utils

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/cppla/aibbs/config"
)

// SendMail sends a plain text email using SMTP settings from config.
func SendMail(to, subject, body string) error {
	cfg := config.Get()
	if cfg.SMTPHost == "" || cfg.SMTPFrom == "" {
		return fmt.Errorf("smtp not configured")
	}
	addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort))
	auth := smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)

	fromName := cfg.SMTPFromName
	if fromName == "" {
		fromName = "AIBBS"
	}
	fromHeader := fmt.Sprintf("%s <%s>", encodeRFC2047(fromName), cfg.SMTPFrom)

	headers := map[string]string{
		"From":         fromHeader,
		"To":           to,
		"Subject":      encodeRFC2047(subject),
		"MIME-Version": "1.0",
		"Content-Type": "text/plain; charset=UTF-8",
	}
	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(body)

	if cfg.SMTPTLS {
		// STARTTLS with timeouts
		d := net.Dialer{Timeout: 5 * time.Second}
		conn, err := d.Dial("tcp", addr)
		if err != nil {
			return err
		}
		// ensure we don't hang forever
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
		host, _, _ := net.SplitHostPort(addr)
		c, err := smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return err
		}
		defer c.Close()
		// STARTTLS if supported
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return err
			}
		}
		if cfg.SMTPUsername != "" {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
		if err := c.Mail(cfg.SMTPFrom); err != nil {
			return err
		}
		if err := c.Rcpt(to); err != nil {
			return err
		}
		wc, err := c.Data()
		if err != nil {
			return err
		}
		if _, err := wc.Write([]byte(msg.String())); err != nil {
			_ = wc.Close()
			return err
		}
		return wc.Close()
	}

	// Plain SMTP without TLS (not recommended)
	return smtp.SendMail(addr, auth, cfg.SMTPFrom, []string{to}, []byte(msg.String()))
}

// encodeRFC2047 encodes a string for non-ASCII mail headers
func encodeRFC2047(s string) string {
	// simplistic: wrap in RFC2047 Q encoding when non-ASCII present
	for i := 0; i < len(s); i++ {
		if s[i] >= 128 {
			return fmt.Sprintf("=?UTF-8?B?%s?=", b64(s))
		}
	}
	return s
}

func b64(s string) string {
	// local small base64 to avoid extra deps
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := []byte(s)
	var out strings.Builder
	for i := 0; i < len(b); i += 3 {
		var v uint32
		n := 0
		for j := 0; j < 3; j++ {
			if i+j < len(b) {
				v = (v << 8) | uint32(b[i+j])
				n++
			} else {
				v <<= 8
			}
		}
		for j := 0; j < 4; j++ {
			if j <= n {
				idx := (v >> (18 - 6*j)) & 0x3F
				out.WriteByte(enc[idx])
			} else {
				out.WriteByte('=')
			}
		}
	}
	return out.String()
}
