package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"euromni/internal/dbdialect"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials       = errors.New("invalid credentials")
	ErrAuthUnavailable          = errors.New("staff authentication is unavailable")
	ErrStaffAlreadyExists       = errors.New("staff account already exists")
	ErrStaffAlreadyBootstrapped = errors.New("shop already has a staff member")
	ErrShopNotFound             = errors.New("shop not found")
)

// StaffSession represents an authenticated staff member for the CMS.
type StaffSession struct {
	ID        string
	ShopID    string
	UserID    string
	Email     string
	Name      string
	Role      string
	ExpiresAt time.Time
	Token     string
}

func (s *Store) AuthenticateStaff(ctx context.Context, shopID, email, password string) (*StaffSession, error) {
	if s == nil || s.SQL == nil {
		return nil, ErrInvalidCredentials
	}

	var userID, name, role string
	var storedHash sql.NullString
	err := s.SQL.QueryRowContext(ctx, `
		SELECT u.id::text, u.password_hash, u.name, sm.role::text
		FROM users u
		JOIN shop_members sm ON sm.user_id = u.id
		WHERE sm.shop_id = $1 AND lower(u.email) = lower($2)
	`, shopID, email).Scan(&userID, &storedHash, &name, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !storedHash.Valid || bcrypt.CompareHashAndPassword([]byte(storedHash.String), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueStaffSession(ctx, shopID, userID, email, name, role)
}

func (s *Store) ValidateStaffSession(ctx context.Context, token string) (*StaffSession, error) {
	if s == nil || s.SQL == nil || token == "" {
		return nil, ErrInvalidCredentials
	}
	hash := sha256.Sum256([]byte(token))
	var sessionID, shopID, userID, email, name, role string
	var expiresAt time.Time
	err := s.SQL.QueryRowContext(ctx, `
		SELECT s.id::text, s.shop_id::text, s.user_id::text, u.email, u.name, sm.role::text, s.expires_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN shop_members sm ON sm.user_id = u.id AND sm.shop_id = s.shop_id
		WHERE s.token_hash = $1 AND s.expires_at > now()
	`, hash[:]).Scan(&sessionID, &shopID, &userID, &email, &name, &role, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	return &StaffSession{ID: sessionID, ShopID: shopID, UserID: userID, Email: email, Name: name, Role: role, ExpiresAt: expiresAt, Token: token}, nil
}

func (s *Store) RevokeStaffSession(ctx context.Context, token string) error {
	if s == nil || s.SQL == nil || token == "" {
		return ErrInvalidCredentials
	}
	hash := sha256.Sum256([]byte(token))
	result, err := s.SQL.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE token_hash = $1 AND user_id IS NOT NULL
	`, hash[:])
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrInvalidCredentials
	}
	return nil
}

func (s *Store) BootstrapStaff(ctx context.Context, shopID, email, name, password string) error {
	if s == nil || s.SQL == nil {
		return ErrAuthUnavailable
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(name)

	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var lockedShopID string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM shops WHERE id = $1 FOR UPDATE`, shopID).Scan(&lockedShopID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShopNotFound
		}
		return err
	}
	var memberCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM shop_members WHERE shop_id = $1`, shopID).Scan(&memberCount); err != nil {
		return err
	}
	if memberCount != 0 {
		if memberCount != 1 {
			return ErrStaffAlreadyBootstrapped
		}
		var userID, existingEmail, role string
		var storedHash sql.NullString
		if err := tx.QueryRowContext(ctx, `
			SELECT u.id::text, u.email::text, sm.role::text, u.password_hash
			FROM shop_members sm
			JOIN users u ON u.id = sm.user_id
			WHERE sm.shop_id = $1
			FOR UPDATE OF u
		`, shopID).Scan(&userID, &existingEmail, &role, &storedHash); err != nil {
			return err
		}
		if role != "owner" || storedHash.Valid || !strings.EqualFold(existingEmail, email) {
			return ErrStaffAlreadyBootstrapped
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE users SET password_hash = $2, name = $3, updated_at = now()
			WHERE id = $1
		`, userID, string(passwordHash), name); err != nil {
			return err
		}
		return tx.Commit()
	}

	userID, err := NewCatalogID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name)
		VALUES ($1, $2, $3, $4)
	`, userID, email, string(passwordHash), name)
	if err != nil {
		if dbdialect.IsUniqueConstraint(err) {
			return ErrStaffAlreadyExists
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO shop_members (shop_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, shopID, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) issueStaffSession(ctx context.Context, shopID, userID, email, name, role string) (*StaffSession, error) {
	token, err := generateSessionToken()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(token))
	expiresAt := time.Now().Add(24 * time.Hour).UTC()

	result, err := s.SQL.ExecContext(ctx, `
		INSERT INTO sessions (shop_id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, shopID, userID, hash[:], expiresAt)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return nil, fmt.Errorf("failed to create staff session")
	}

	var sessionID string
	if err := s.SQL.QueryRowContext(ctx, `SELECT id::text FROM sessions WHERE token_hash = $1`, hash[:]).Scan(&sessionID); err != nil {
		return nil, err
	}

	return &StaffSession{ID: sessionID, ShopID: shopID, UserID: userID, Email: email, Name: name, Role: role, ExpiresAt: expiresAt, Token: token}, nil
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
