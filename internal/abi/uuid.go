package abi

import (
	"fmt"
	"strings"
)

func NormalizeUUID(s string) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) != 36 {
		return "", fmt.Errorf("uuid")
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return "", fmt.Errorf("uuid")
			}
		default:
			if !isHex(r) {
				return "", fmt.Errorf("uuid")
			}
		}
	}
	return s, nil
}

func isHex(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}
