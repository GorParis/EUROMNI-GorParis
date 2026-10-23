package mailer

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"
)

type TLSMode string

const (
	TLSStartTLS TLSMode = "starttls"
	TLSImplicit TLSMode = "implicit"
	TLSDisabled TLSMode = "none"
)

type Config struct {
	Address      string
	From         mail.Address
	Username     string
	Password     string
	TLSMode      TLSMode
	Environment  string
	PollInterval time.Duration
	Timeout      time.Duration
}

func ConfigFromEnv() (Config, error) {
	address := strings.TrimSpace(os.Getenv("EUROMNI_MAIL_SMTP_ADDR"))
	fromValue := strings.TrimSpace(os.Getenv("EUROMNI_MAIL_FROM"))
	if address == "" || fromValue == "" {
		return Config{}, errors.New("EUROMNI_MAIL_SMTP_ADDR and EUROMNI_MAIL_FROM are required")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" {
		return Config{}, errors.New("EUROMNI_MAIL_SMTP_ADDR must be host:port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, errors.New("EUROMNI_MAIL_SMTP_ADDR has an invalid port")
	}
	from, err := mail.ParseAddress(fromValue)
	if err != nil || from.Address == "" {
		return Config{}, errors.New("EUROMNI_MAIL_FROM must be a valid email address")
	}
	username := strings.TrimSpace(os.Getenv("EUROMNI_MAIL_SMTP_USERNAME"))
	password := os.Getenv("EUROMNI_MAIL_SMTP_PASSWORD")
	if (username == "") != (password == "") {
		return Config{}, errors.New("SMTP username and password must be configured together")
	}
	mode := TLSMode(strings.TrimSpace(os.Getenv("EUROMNI_MAIL_TLS_MODE")))
	if mode == "" {
		mode = TLSStartTLS
	}
	switch mode {
	case TLSStartTLS, TLSImplicit:
	case TLSDisabled:
		if strings.TrimSpace(os.Getenv("EUROMNI_ENVIRONMENT")) != "development" {
			return Config{}, errors.New("EUROMNI_MAIL_TLS_MODE=none is allowed only in development")
		}
		if username != "" {
			return Config{}, errors.New("SMTP authentication is not allowed without TLS")
		}
	default:
		return Config{}, fmt.Errorf("unsupported EUROMNI_MAIL_TLS_MODE %q", mode)
	}
	pollInterval := 2 * time.Second
	if value := strings.TrimSpace(os.Getenv("EUROMNI_MAIL_POLL_INTERVAL")); value != "" {
		pollInterval, err = time.ParseDuration(value)
		if err != nil || pollInterval < 250*time.Millisecond || pollInterval > time.Minute {
			return Config{}, errors.New("EUROMNI_MAIL_POLL_INTERVAL must be between 250ms and 1m")
		}
	}
	return Config{
		Address: address, From: *from, Username: username, Password: password,
		TLSMode: mode, Environment: strings.TrimSpace(os.Getenv("EUROMNI_ENVIRONMENT")),
		PollInterval: pollInterval, Timeout: 15 * time.Second,
	}, nil
}
