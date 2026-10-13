package store

import (
	"context"
	"database/sql"
)

type CategoryView struct {
	ID           string                    `json:"id"`
	Slug         string                    `json:"slug"`
	Translations []CategoryTranslationView `json:"translations"`
}

type CategoryTranslationView struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CategoryReader interface {
	ListCategories(ctx context.Context, shopID string) ([]CategoryView, error)
	GetCategory(ctx context.Context, shopID, categoryID string) (*CategoryView, error)
}

func (s *Store) ListCategories(ctx context.Context, shopID string) ([]CategoryView, error) {
	return s.readCategories(ctx, shopID, "")
}

func (s *Store) GetCategory(ctx context.Context, shopID, categoryID string) (*CategoryView, error) {
	categories, err := s.readCategories(ctx, shopID, categoryID)
	if err != nil {
		return nil, err
	}
	if len(categories) == 0 {
		return nil, ErrNotFound
	}
	return &categories[0], nil
}

func (s *Store) readCategories(ctx context.Context, shopID, categoryID string) ([]CategoryView, error) {
	if s == nil || s.SQL == nil {
		return nil, sql.ErrConnDone
	}

	query := `
		SELECT c.id::text, c.slug, ct.locale, ct.name, ct.description
		FROM categories c
		LEFT JOIN category_translations ct ON ct.category_id = c.id
		WHERE c.shop_id = $1
	`
	args := []interface{}{shopID}
	if categoryID != "" {
		query += ` AND c.id::text = $2`
		args = append(args, categoryID)
	}
	query += ` ORDER BY c.position, c.id, ct.locale`

	rows, err := s.SQL.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]CategoryView, 0)
	categoryIndexes := make(map[string]int)
	for rows.Next() {
		var id, slug string
		var locale, name, description sql.NullString
		if err := rows.Scan(&id, &slug, &locale, &name, &description); err != nil {
			return nil, err
		}
		index, ok := categoryIndexes[id]
		if !ok {
			index = len(categories)
			categoryIndexes[id] = index
			categories = append(categories, CategoryView{
				ID:           id,
				Slug:         slug,
				Translations: make([]CategoryTranslationView, 0),
			})
		}
		if locale.Valid {
			categories[index].Translations = append(categories[index].Translations, CategoryTranslationView{
				Locale:      locale.String,
				Name:        name.String,
				Description: description.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return categories, nil
}
