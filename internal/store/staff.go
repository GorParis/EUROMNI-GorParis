package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"euromni/internal/dbdialect"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrStaffOwnerProtected = errors.New("owner membership cannot be changed through staff administration")
	ErrStaffRoleInvalid    = errors.New("invalid staff role")
)

type StaffMemberView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type StaffMemberInput struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password,omitempty"`
}

type StaffMemberWriter interface {
	ListStaffMembers(ctx context.Context, shopID string) ([]StaffMemberView, error)
	CreateStaffMember(ctx context.Context, shopID string, input StaffMemberInput) (*StaffMemberView, error)
	UpdateStaffMember(ctx context.Context, shopID, memberID string, input StaffMemberInput) (*StaffMemberView, error)
	RemoveStaffMember(ctx context.Context, shopID, memberID string) error
}

var _ StaffMemberWriter = (*Store)(nil)

func (s *Store) ListStaffMembers(ctx context.Context, shopID string) ([]StaffMemberView, error) {
	if s == nil || s.SQL == nil {
		return nil, ErrAuthUnavailable
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT u.id::text, u.email::text, u.name, sm.role::text
		FROM shop_members sm
		JOIN users u ON u.id = sm.user_id
		WHERE sm.shop_id = $1
		ORDER BY CASE sm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'editor' THEN 2 ELSE 3 END, lower(u.email)
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]StaffMemberView, 0)
	for rows.Next() {
		var member StaffMemberView
		if err := rows.Scan(&member.ID, &member.Email, &member.Name, &member.Role); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}

func (s *Store) CreateStaffMember(ctx context.Context, shopID string, input StaffMemberInput) (*StaffMemberView, error) {
	if s == nil || s.SQL == nil {
		return nil, ErrAuthUnavailable
	}
	if !assignableStaffRole(input.Role) {
		return nil, ErrStaffRoleInvalid
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash staff password: %w", err)
	}
	email := strings.TrimSpace(input.Email)
	name := strings.TrimSpace(input.Name)

	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	memberID, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	member := StaffMemberView{ID: memberID, Email: email, Name: name}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name)
		VALUES ($1, $2, $3, $4)
	`, member.ID, email, string(passwordHash), name)
	if err != nil {
		if isUniqueConstraint(err) {
			return nil, ErrStaffAlreadyExists
		}
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO shop_members (shop_id, user_id, role)
		VALUES ($1, $2, $3)
	`, shopID, member.ID, input.Role); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	member.Role = input.Role
	return &member, nil
}

func (s *Store) UpdateStaffMember(ctx context.Context, shopID, memberID string, input StaffMemberInput) (*StaffMemberView, error) {
	if s == nil || s.SQL == nil {
		return nil, ErrAuthUnavailable
	}
	if !assignableStaffRole(input.Role) {
		return nil, ErrStaffRoleInvalid
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var member StaffMemberView
	err = tx.QueryRowContext(ctx, `
		SELECT u.id::text, u.email::text, u.name, sm.role::text
		FROM shop_members sm JOIN users u ON u.id = sm.user_id
		WHERE sm.shop_id = $1 AND sm.user_id = $2
		FOR UPDATE OF sm, u
	`, shopID, memberID).Scan(&member.ID, &member.Email, &member.Name, &member.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if member.Role == "owner" || input.Role == "owner" {
		return nil, ErrStaffOwnerProtected
	}
	passwordHash := ""
	if input.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash staff password: %w", err)
		}
		passwordHash = string(hash)
	}
	member.Name = strings.TrimSpace(input.Name)
	member.Role = input.Role
	if passwordHash == "" {
		_, err = tx.ExecContext(ctx, `
			UPDATE users SET name = $2, updated_at = now() WHERE id = $1
		`, memberID, member.Name)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE users SET name = $2, password_hash = $3, updated_at = now() WHERE id = $1
		`, memberID, member.Name, passwordHash)
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE shop_members SET role = $3 WHERE shop_id = $1 AND user_id = $2
	`, shopID, memberID, member.Role); err != nil {
		return nil, err
	}
	if passwordHash != "" {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM sessions WHERE shop_id = $1 AND user_id = $2
		`, shopID, memberID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &member, nil
}

func (s *Store) RemoveStaffMember(ctx context.Context, shopID, memberID string) error {
	if s == nil || s.SQL == nil {
		return ErrAuthUnavailable
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var role string
	err = tx.QueryRowContext(ctx, `
		SELECT role::text FROM shop_members
		WHERE shop_id = $1 AND user_id = $2
		FOR UPDATE
	`, shopID, memberID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == "owner" {
		return ErrStaffOwnerProtected
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM sessions WHERE shop_id = $1 AND user_id = $2
	`, shopID, memberID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM shop_members WHERE shop_id = $1 AND user_id = $2
	`, shopID, memberID); err != nil {
		return err
	}
	return tx.Commit()
}

func assignableStaffRole(role string) bool {
	return role == "admin" || role == "editor" || role == "support"
}

func isUniqueConstraint(err error) bool {
	return dbdialect.IsUniqueConstraint(err)
}
