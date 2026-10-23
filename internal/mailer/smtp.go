package mailer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

type Sender interface {
	Send(context.Context, Message) error
}

type SMTPSender struct {
	config Config
}

func NewSMTPSender(config Config) (*SMTPSender, error) {
	if config.Address == "" || config.From.Address == "" || config.Timeout <= 0 {
		return nil, errors.New("invalid SMTP sender configuration")
	}
	if _, _, err := net.SplitHostPort(config.Address); err != nil {
		return nil, errors.New("invalid SMTP server address")
	}
	if config.TLSMode != TLSStartTLS && config.TLSMode != TLSImplicit && config.TLSMode != TLSDisabled {
		return nil, errors.New("unsupported SMTP TLS mode")
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, errors.New("SMTP username and password must be configured together")
	}
	if config.TLSMode == TLSDisabled &&
		(config.Environment != "development" || config.Username != "" || config.Password != "") {
		return nil, errors.New("unencrypted SMTP is restricted to unauthenticated development use")
	}
	if _, err := mail.ParseAddress(config.From.String()); err != nil {
		return nil, errors.New("invalid SMTP sender address")
	}
	return &SMTPSender{config: config}, nil
}

func (s *SMTPSender) Send(ctx context.Context, message Message) error {
	if s == nil || ctx == nil {
		return errors.New("SMTP sender is unavailable")
	}
	from, err := mail.ParseAddress(s.config.From.String())
	if err != nil {
		return fmt.Errorf("parse SMTP sender address: %w", err)
	}
	to, err := mail.ParseAddress(message.To)
	if err != nil {
		return fmt.Errorf("parse recipient address: %w", err)
	}
	host, _, err := net.SplitHostPort(s.config.Address)
	if err != nil {
		return errors.New("invalid SMTP server address")
	}
	dialer := net.Dialer{Timeout: s.config.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", s.config.Address)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer func() {
		stopClose()
		_ = conn.Close()
	}()
	deadline := time.Now().Add(s.config.Timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}
	if s.config.TLSMode == TLSImplicit {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("establish SMTP TLS: %w", err)
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("start SMTP session: %w", err)
	}
	defer func() { _ = client.Close() }()
	switch s.config.TLSMode {
	case TLSStartTLS:
		supported, _ := client.Extension("STARTTLS")
		if !supported {
			return errors.New("SMTP server does not offer required STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("establish SMTP STARTTLS: %w", err)
		}
	case TLSImplicit, TLSDisabled:
	default:
		return errors.New("unsupported SMTP TLS mode")
	}
	if s.config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.config.Username, s.config.Password, host)); err != nil {
			return fmt.Errorf("authenticate with SMTP server: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	data, err := client.Data()
	if err != nil {
		return fmt.Errorf("start SMTP message: %w", err)
	}
	encoded, err := encodeMessage(s.config.From, *to, message)
	if err != nil {
		_ = data.Close()
		return err
	}
	if _, err := data.Write(encoded); err != nil {
		_ = data.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := data.Close(); err != nil {
		return fmt.Errorf("submit SMTP message: %w", err)
	}
	return nil
}

func encodeMessage(from, to mail.Address, message Message) ([]byte, error) {
	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	writePart := func(contentType, content string) error {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", contentType+"; charset=UTF-8")
		header.Set("Content-Transfer-Encoding", "quoted-printable")
		part, err := multipartWriter.CreatePart(header)
		if err != nil {
			return err
		}
		writer := quotedprintable.NewWriter(part)
		if _, err := io.WriteString(writer, content); err != nil {
			_ = writer.Close()
			return err
		}
		return writer.Close()
	}
	if err := writePart("text/plain", message.TextBody); err != nil {
		return nil, fmt.Errorf("encode text email part: %w", err)
	}
	if err := writePart("text/html", message.HTMLBody); err != nil {
		return nil, fmt.Errorf("encode HTML email part: %w", err)
	}
	if err := multipartWriter.Close(); err != nil {
		return nil, fmt.Errorf("finish email MIME body: %w", err)
	}

	var output strings.Builder
	fmt.Fprintf(&output, "From: %s\r\n", from.String())
	fmt.Fprintf(&output, "To: %s\r\n", to.String())
	fmt.Fprintf(&output, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", message.Subject))
	fmt.Fprintf(&output, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	messageIDHash := sha256.Sum256([]byte(message.Idempotency))
	fromDomain := from.Address[strings.LastIndex(from.Address, "@")+1:]
	fmt.Fprintf(&output, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(messageIDHash[:]), fromDomain)
	output.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&output, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", multipartWriter.Boundary())
	output.Write(body.Bytes())
	return []byte(output.String()), nil
}

var _ Sender = (*SMTPSender)(nil)
