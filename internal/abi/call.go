package abi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	Version      = "euromni.abi.v1"
	MaxCallBytes = 16 << 10 // 16 KiB — the guest never streams SQL
	MaxCorrIDLen = 36
)

type Envelope struct {
	ABI   string          `json:"abi"`
	Call  string          `json:"call"`
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
}

type Response struct {
	OK    bool       `json:"ok"`
	ABI   string     `json:"abi"`
	Call  string     `json:"call,omitempty"`
	ID    string     `json:"id,omitempty"`
	Data  any        `json:"data,omitempty"`
	Error *ErrorBody `json:"error,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	CodeInvalid       = "invalid"
	CodeDenied        = "denied"
	CodeNotFound      = "not_found"
	CodeUnimplemented = "unimplemented"
	CodeInternal      = "internal"
	CodeTooLarge      = "too_large"
)

type GuestError struct {
	Code    string
	Message string
}

func (e *GuestError) Error() string { return e.Code + ": " + e.Message }

func Fail(code, message string) *GuestError {
	return &GuestError{Code: code, Message: message}
}

func ParseEnvelope(raw []byte) (*Envelope, error) {
	if len(raw) == 0 {
		return nil, Fail(CodeInvalid, "empty call")
	}
	if len(raw) > MaxCallBytes {
		return nil, Fail(CodeTooLarge, "call exceeds size limit")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var env Envelope
	if err := dec.Decode(&env); err != nil {
		return nil, Fail(CodeInvalid, "malformed call")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, Fail(CodeInvalid, "malformed call")
	}
	if env.ABI != Version {
		return nil, Fail(CodeInvalid, "unsupported abi")
	}
	env.Call = strings.TrimSpace(env.Call)
	if env.Call == "" || !validCallName(env.Call) {
		return nil, Fail(CodeInvalid, "invalid call")
	}
	if env.ID != "" {
		if len(env.ID) > MaxCorrIDLen {
			return nil, Fail(CodeInvalid, "invalid id")
		}
		id, err := NormalizeUUID(env.ID)
		if err != nil {
			return nil, Fail(CodeInvalid, "invalid id")
		}
		env.ID = id
	}
	if len(env.Input) == 0 || string(env.Input) == "null" {
		return nil, Fail(CodeInvalid, "missing input")
	}
	if env.Input[0] != '{' {
		return nil, Fail(CodeInvalid, "input must be an object")
	}
	return &env, nil
}

func DecodeInput[T any](raw json.RawMessage) (T, error) {
	var zero T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var v T
	if err := dec.Decode(&v); err != nil {
		return zero, Fail(CodeInvalid, "invalid input")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return zero, Fail(CodeInvalid, "invalid input")
	}
	return v, nil
}

func validCallName(s string) bool {
	if len(s) > 64 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '.' && i > 0 && i < len(s)-1:
		default:
			return false
		}
	}
	return strings.Contains(s, ".")
}

func EncodeOK(call, id string, data any) []byte {
	b, err := json.Marshal(Response{
		OK:   true,
		ABI:  Version,
		Call: call,
		ID:   id,
		Data: data,
	})
	if err != nil {
		return EncodeErr(call, id, CodeInternal, "request failed")
	}
	return b
}

func EncodeErr(call, id, code, message string) []byte {
	b, err := json.Marshal(Response{
		OK:   false,
		ABI:  Version,
		Call: call,
		ID:   id,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
		},
	})
	if err != nil {
		return []byte(`{"ok":false,"abi":"` + Version + `","error":{"code":"internal","message":"request failed"}}`)
	}
	return b
}

func GuestCode(err error) (code, message string) {
	var g *GuestError
	if errors.As(err, &g) {
		return g.Code, g.Message
	}
	return CodeInternal, "request failed"
}

func RequireUUID(field, value string) (string, error) {
	id, err := NormalizeUUID(value)
	if err != nil {
		return "", Fail(CodeInvalid, fmt.Sprintf("invalid %s", field))
	}
	return id, nil
}
