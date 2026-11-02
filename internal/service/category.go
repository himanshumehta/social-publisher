package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type CategoryService struct {
	pool *pgxpool.Pool
}

func NewCategoryService(pool *pgxpool.Pool) *CategoryService {
	return &CategoryService{pool: pool}
}

func (s *CategoryService) List(ctx context.Context) ([]model.Category, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, COALESCE(description, ''), COALESCE(icon_url, ''), is_active, display_order, created_at, updated_at
		 FROM categories WHERE is_active = TRUE ORDER BY display_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.IconURL,
			&c.IsActive, &c.DisplayOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating categories: %w", err)
	}
	return categories, nil
}
