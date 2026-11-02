package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type PortfolioService struct {
	pool *pgxpool.Pool
}

func NewPortfolioService(pool *pgxpool.Pool) *PortfolioService {
	return &PortfolioService{pool: pool}
}

// ── Scan helpers ────────────────────────────────────────────────────────

const portfolioColumns = `id, editor_id, bio, specializations, years_experience,
	hourly_rate, availability, portfolio_url, demo_reel_url, is_featured,
	created_at, updated_at`

func scanPortfolio(row pgx.Row) (*model.EditorPortfolio, error) {
	var p model.EditorPortfolio
	err := row.Scan(
		&p.ID, &p.EditorID, &p.Bio, &p.Specializations, &p.YearsExperience,
		&p.HourlyRate, &p.Availability, &p.PortfolioURL, &p.DemoReelURL,
		&p.IsFeatured, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

const portfolioItemColumns = `id, portfolio_id, title, description, thumbnail_url,
	media_url, media_type, platform, view_count, sort_order, created_at`

func scanPortfolioItem(row pgx.Row) (*model.PortfolioItem, error) {
	var item model.PortfolioItem
	err := row.Scan(
		&item.ID, &item.PortfolioID, &item.Title, &item.Description,
		&item.ThumbnailURL, &item.MediaURL, &item.MediaType, &item.Platform,
		&item.ViewCount, &item.SortOrder, &item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

const skillColumns = `id, editor_id, skill_name, proficiency, verified`

func scanSkill(row pgx.Row) (*model.EditorSkill, error) {
	var s model.EditorSkill
	err := row.Scan(&s.ID, &s.EditorID, &s.SkillName, &s.Proficiency, &s.Verified)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

const mktTemplateColumns = `id, creator_id, title, description, category,
	brief_template, price, purchase_count, rating_avg, is_active,
	created_at, updated_at`

func scanMktTemplate(row pgx.Row) (*model.MarketplaceTemplate, error) {
	var t model.MarketplaceTemplate
	err := row.Scan(
		&t.ID, &t.CreatorID, &t.Title, &t.Description, &t.Category,
		&t.BriefTemplate, &t.Price, &t.PurchaseCount, &t.RatingAvg,
		&t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ── Availability validation ─────────────────────────────────────────────

var validAvailabilities = map[string]bool{
	"available":   true,
	"busy":        true,
	"unavailable": true,
}

var validMediaTypes = map[string]bool{
	"video": true,
	"image": true,
	"link":  true,
}

var validProficiencies = map[string]bool{
	"beginner":     true,
	"intermediate": true,
	"advanced":     true,
	"expert":       true,
}

// ── Portfolio CRUD ──────────────────────────────────────────────────────

func (s *PortfolioService) CreatePortfolio(ctx context.Context, editorID uuid.UUID, req *model.CreatePortfolioRequest) (*model.EditorPortfolio, error) {
	availability := req.Availability
	if availability == "" {
		availability = "available"
	}
	if !validAvailabilities[availability] {
		return nil, fmt.Errorf("invalid availability: %s", availability)
	}

	specializations := req.Specializations
	if specializations == nil {
		specializations = []string{}
	}

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO editor_portfolios
			(editor_id, bio, specializations, years_experience, hourly_rate,
			 availability, portfolio_url, demo_reel_url)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING %s`, portfolioColumns),
		editorID, req.Bio, specializations, req.YearsExperience,
		req.HourlyRate, availability, req.PortfolioURL, req.DemoReelURL,
	)
	p, err := scanPortfolio(row)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("portfolio already exists for this editor")
		}
		return nil, fmt.Errorf("create portfolio: %w", err)
	}

	p.Skills = []model.EditorSkill{}
	p.Items = []model.PortfolioItem{}
	return p, nil
}

func (s *PortfolioService) GetPortfolio(ctx context.Context, editorID uuid.UUID) (*model.EditorPortfolio, error) {
	// Load portfolio
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM editor_portfolios WHERE editor_id = $1`, portfolioColumns),
		editorID,
	)
	p, err := scanPortfolio(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("portfolio not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get portfolio: %w", err)
	}

	// Load skills
	skillRows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM editor_skills WHERE editor_id = $1 ORDER BY skill_name`, skillColumns),
		editorID,
	)
	if err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	defer skillRows.Close()

	p.Skills = []model.EditorSkill{}
	for skillRows.Next() {
		sk, err := scanSkill(skillRows)
		if err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		p.Skills = append(p.Skills, *sk)
	}

	// Load items
	itemRows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM portfolio_items WHERE portfolio_id = $1
			ORDER BY sort_order ASC, created_at DESC`, portfolioItemColumns),
		p.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("load items: %w", err)
	}
	defer itemRows.Close()

	p.Items = []model.PortfolioItem{}
	for itemRows.Next() {
		item, err := scanPortfolioItem(itemRows)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		p.Items = append(p.Items, *item)
	}

	return p, nil
}

func (s *PortfolioService) UpdatePortfolio(ctx context.Context, editorID uuid.UUID, req *model.UpdatePortfolioRequest) (*model.EditorPortfolio, error) {
	// Fetch existing
	existing, err := s.GetPortfolio(ctx, editorID)
	if err != nil {
		return nil, err
	}

	if req.Bio != nil {
		existing.Bio = *req.Bio
	}
	if req.Specializations != nil {
		existing.Specializations = *req.Specializations
	}
	if req.YearsExperience != nil {
		existing.YearsExperience = *req.YearsExperience
	}
	if req.HourlyRate != nil {
		existing.HourlyRate = *req.HourlyRate
	}
	if req.Availability != nil {
		if !validAvailabilities[*req.Availability] {
			return nil, fmt.Errorf("invalid availability: %s", *req.Availability)
		}
		existing.Availability = model.AvailabilityStatus(*req.Availability)
	}
	if req.PortfolioURL != nil {
		existing.PortfolioURL = *req.PortfolioURL
	}
	if req.DemoReelURL != nil {
		existing.DemoReelURL = *req.DemoReelURL
	}

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE editor_portfolios SET
			bio = $1, specializations = $2, years_experience = $3, hourly_rate = $4,
			availability = $5, portfolio_url = $6, demo_reel_url = $7, updated_at = NOW()
			WHERE editor_id = $8
			RETURNING %s`, portfolioColumns),
		existing.Bio, existing.Specializations, existing.YearsExperience,
		existing.HourlyRate, existing.Availability, existing.PortfolioURL,
		existing.DemoReelURL, editorID,
	)

	updated, err := scanPortfolio(row)
	if err != nil {
		return nil, fmt.Errorf("update portfolio: %w", err)
	}

	// Carry over loaded associations
	updated.Skills = existing.Skills
	updated.Items = existing.Items
	return updated, nil
}

// ── Portfolio Items ─────────────────────────────────────────────────────

func (s *PortfolioService) AddPortfolioItem(ctx context.Context, editorID uuid.UUID, req *model.AddPortfolioItemRequest) (*model.PortfolioItem, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = "video"
	}
	if !validMediaTypes[mediaType] {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}

	// Look up portfolio ID
	var portfolioID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM editor_portfolios WHERE editor_id = $1`, editorID,
	).Scan(&portfolioID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("portfolio not found; create a portfolio first")
	}
	if err != nil {
		return nil, fmt.Errorf("lookup portfolio: %w", err)
	}

	// Get next sort order
	var maxSort int
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(sort_order), 0) FROM portfolio_items WHERE portfolio_id = $1`,
		portfolioID,
	).Scan(&maxSort)

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO portfolio_items
			(portfolio_id, title, description, thumbnail_url, media_url, media_type, platform, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING %s`, portfolioItemColumns),
		portfolioID, req.Title, req.Description, req.ThumbnailURL,
		req.MediaURL, mediaType, req.Platform, maxSort+1,
	)
	return scanPortfolioItem(row)
}

func (s *PortfolioService) RemovePortfolioItem(ctx context.Context, editorID, itemID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM portfolio_items
		 WHERE id = $1 AND portfolio_id = (
		     SELECT id FROM editor_portfolios WHERE editor_id = $2
		 )`, itemID, editorID,
	)
	if err != nil {
		return fmt.Errorf("remove portfolio item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("portfolio item not found or not owned by you")
	}
	return nil
}

func (s *PortfolioService) ReorderPortfolioItems(ctx context.Context, editorID uuid.UUID, itemIDs []uuid.UUID) error {
	// Look up portfolio ID
	var portfolioID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM editor_portfolios WHERE editor_id = $1`, editorID,
	).Scan(&portfolioID)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("portfolio not found")
	}
	if err != nil {
		return fmt.Errorf("lookup portfolio: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for i, itemID := range itemIDs {
		tag, err := tx.Exec(ctx,
			`UPDATE portfolio_items SET sort_order = $1
			 WHERE id = $2 AND portfolio_id = $3`,
			i, itemID, portfolioID,
		)
		if err != nil {
			return fmt.Errorf("update sort order: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("item %s not found in portfolio", itemID)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ── Skills ──────────────────────────────────────────────────────────────

func (s *PortfolioService) SetSkills(ctx context.Context, editorID uuid.UUID, req *model.SetSkillsRequest) ([]model.EditorSkill, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete existing skills
	_, err = tx.Exec(ctx, `DELETE FROM editor_skills WHERE editor_id = $1`, editorID)
	if err != nil {
		return nil, fmt.Errorf("clear skills: %w", err)
	}

	results := []model.EditorSkill{}
	for _, skill := range req.Skills {
		if skill.Name == "" {
			continue
		}
		proficiency := skill.Proficiency
		if proficiency == "" {
			proficiency = "intermediate"
		}
		if !validProficiencies[proficiency] {
			return nil, fmt.Errorf("invalid proficiency: %s", proficiency)
		}

		var sk model.EditorSkill
		err := tx.QueryRow(ctx,
			fmt.Sprintf(`INSERT INTO editor_skills (editor_id, skill_name, proficiency)
				VALUES ($1, $2, $3)
				RETURNING %s`, skillColumns),
			editorID, skill.Name, proficiency,
		).Scan(&sk.ID, &sk.EditorID, &sk.SkillName, &sk.Proficiency, &sk.Verified)
		if err != nil {
			return nil, fmt.Errorf("insert skill %q: %w", skill.Name, err)
		}
		results = append(results, sk)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return results, nil
}

// ── Editor Search ───────────────────────────────────────────────────────

func (s *PortfolioService) SearchEditors(ctx context.Context, filter *model.EditorSearchFilter) ([]model.EditorSearchResult, int, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Build dynamic WHERE clause
	conditions := []string{"ep.id IS NOT NULL"}
	args := []interface{}{}
	argIdx := 1

	if filter.Availability != "" {
		conditions = append(conditions, fmt.Sprintf("ep.availability = $%d", argIdx))
		args = append(args, filter.Availability)
		argIdx++
	}

	if filter.MaxRate > 0 {
		conditions = append(conditions, fmt.Sprintf("ep.hourly_rate <= $%d", argIdx))
		args = append(args, filter.MaxRate)
		argIdx++
	}

	if filter.MinRating > 0 {
		conditions = append(conditions, fmt.Sprintf(
			`COALESCE((SELECT AVG(rating)::float8 FROM editor_ratings WHERE rated_user = ep.editor_id), 0) >= $%d`,
			argIdx))
		args = append(args, filter.MinRating)
		argIdx++
	}

	if len(filter.Skills) > 0 {
		conditions = append(conditions, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM editor_skills es2 WHERE es2.editor_id = ep.editor_id AND es2.skill_name = ANY($%d))`,
			argIdx))
		args = append(args, filter.Skills)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total matches
	countQuery := fmt.Sprintf(
		`SELECT COUNT(DISTINCT ep.editor_id) FROM editor_portfolios ep WHERE %s`, whereClause)

	var total int
	err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count editors: %w", err)
	}

	// Fetch paginated results
	dataQuery := fmt.Sprintf(
		`SELECT
			ep.editor_id,
			u.name,
			COALESCE(u.avatar_url, '') AS avatar_url,
			ep.bio,
			ep.hourly_rate,
			ep.availability,
			COALESCE((SELECT AVG(rating)::float8 FROM editor_ratings WHERE rated_user = ep.editor_id), 0) AS avg_rating,
			COALESCE((SELECT COUNT(*) FROM editing_assignments WHERE editor_id = ep.editor_id AND status = 'completed'), 0) AS completed_jobs
		FROM editor_portfolios ep
		JOIN users u ON u.id = ep.editor_id
		WHERE %s
		ORDER BY avg_rating DESC, completed_jobs DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, argIdx, argIdx+1)

	args = append(args, pageSize, offset)

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search editors: %w", err)
	}
	defer rows.Close()

	results := []model.EditorSearchResult{}
	for rows.Next() {
		var r model.EditorSearchResult
		err := rows.Scan(
			&r.EditorID, &r.Name, &r.AvatarURL, &r.Bio,
			&r.HourlyRate, &r.Availability, &r.AvgRating, &r.CompletedJobs,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan editor result: %w", err)
		}

		// Load skills for each editor
		skillRows, err := s.pool.Query(ctx,
			fmt.Sprintf(`SELECT %s FROM editor_skills WHERE editor_id = $1 ORDER BY skill_name`, skillColumns),
			r.EditorID,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("load editor skills: %w", err)
		}
		r.Skills = []model.EditorSkill{}
		for skillRows.Next() {
			sk, err := scanSkill(skillRows)
			if err != nil {
				skillRows.Close()
				return nil, 0, fmt.Errorf("scan editor skill: %w", err)
			}
			r.Skills = append(r.Skills, *sk)
		}
		skillRows.Close()

		// Calculate match score if skill filter is provided
		if len(filter.Skills) > 0 {
			r.MatchScore = calculateMatchScore(filter.Skills, r.Skills, r.AvgRating)
		}

		results = append(results, r)
	}

	// Sort by match score descending if skill filter was applied
	if len(filter.Skills) > 0 {
		sort.Slice(results, func(i, j int) bool {
			return results[i].MatchScore > results[j].MatchScore
		})
	}

	return results, total, nil
}

// ── Job Matching ────────────────────────────────────────────────────────

func (s *PortfolioService) MatchEditorsForJob(ctx context.Context, jobID uuid.UUID) ([]model.EditorSearchResult, error) {
	// Load the job to get its brief/title for keyword extraction
	var jobTitle, jobBrief string
	var jobComplexity string
	err := s.pool.QueryRow(ctx,
		`SELECT title, brief, complexity FROM editing_jobs WHERE id = $1`, jobID,
	).Scan(&jobTitle, &jobBrief, &jobComplexity)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("editing job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	// Extract skill keywords from job title and brief
	// We match against existing skills in the system
	jobText := strings.ToLower(jobTitle + " " + jobBrief)

	// Get all unique skill names from editor_skills
	skillRows, err := s.pool.Query(ctx,
		`SELECT DISTINCT skill_name FROM editor_skills`)
	if err != nil {
		return nil, fmt.Errorf("get skill names: %w", err)
	}
	defer skillRows.Close()

	var jobSkills []string
	for skillRows.Next() {
		var name string
		if err := skillRows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan skill name: %w", err)
		}
		if strings.Contains(jobText, strings.ToLower(name)) {
			jobSkills = append(jobSkills, name)
		}
	}

	// Search editors with available status
	results, _, err := s.SearchEditors(ctx, &model.EditorSearchFilter{
		Skills:       jobSkills,
		Availability: "available",
		Page:         1,
		PageSize:     50,
	})
	if err != nil {
		return nil, fmt.Errorf("search editors for job: %w", err)
	}

	// Re-score with job-specific skills
	for i := range results {
		results[i].MatchScore = calculateMatchScore(jobSkills, results[i].Skills, results[i].AvgRating)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].MatchScore > results[j].MatchScore
	})

	return results, nil
}

// ── Marketplace Templates ───────────────────────────────────────────────

func (s *PortfolioService) CreateMarketplaceTemplate(ctx context.Context, creatorID uuid.UUID, req *model.CreateMarketplaceTemplateRequest) (*model.MarketplaceTemplate, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if req.Category == "" {
		return nil, fmt.Errorf("category is required")
	}
	if req.BriefTemplate == "" {
		return nil, fmt.Errorf("brief_template is required")
	}
	if req.Price < 0 {
		return nil, fmt.Errorf("price must be non-negative")
	}

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO marketplace_templates
			(creator_id, title, description, category, brief_template, price)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING %s`, mktTemplateColumns),
		creatorID, req.Title, req.Description, req.Category,
		req.BriefTemplate, req.Price,
	)
	return scanMktTemplate(row)
}

func (s *PortfolioService) ListMarketplaceTemplates(ctx context.Context, category string, page, pageSize int) ([]model.MarketplaceTemplate, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	var countQuery string
	var countArgs []interface{}

	if category != "" {
		countQuery = `SELECT COUNT(*) FROM marketplace_templates WHERE is_active = TRUE AND category = $1`
		countArgs = []interface{}{category}
	} else {
		countQuery = `SELECT COUNT(*) FROM marketplace_templates WHERE is_active = TRUE`
		countArgs = []interface{}{}
	}

	err := s.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count templates: %w", err)
	}

	var dataQuery string
	var dataArgs []interface{}

	if category != "" {
		dataQuery = fmt.Sprintf(`SELECT %s FROM marketplace_templates
			WHERE is_active = TRUE AND category = $1
			ORDER BY purchase_count DESC, rating_avg DESC
			LIMIT $2 OFFSET $3`, mktTemplateColumns)
		dataArgs = []interface{}{category, pageSize, offset}
	} else {
		dataQuery = fmt.Sprintf(`SELECT %s FROM marketplace_templates
			WHERE is_active = TRUE
			ORDER BY purchase_count DESC, rating_avg DESC
			LIMIT $1 OFFSET $2`, mktTemplateColumns)
		dataArgs = []interface{}{pageSize, offset}
	}

	rows, err := s.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	results := []model.MarketplaceTemplate{}
	for rows.Next() {
		t, err := scanMktTemplate(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan template: %w", err)
		}
		results = append(results, *t)
	}
	return results, total, nil
}

func (s *PortfolioService) PurchaseTemplate(ctx context.Context, buyerID, templateID uuid.UUID) (*model.MarketplaceTemplate, error) {
	// Increment purchase count and return updated template
	// TODO: integrate with payment system for templates with price > 0
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE marketplace_templates
			SET purchase_count = purchase_count + 1, updated_at = NOW()
			WHERE id = $1 AND is_active = TRUE
			RETURNING %s`, mktTemplateColumns),
		templateID,
	)
	t, err := scanMktTemplate(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("template not found or inactive")
	}
	if err != nil {
		return nil, fmt.Errorf("purchase template: %w", err)
	}
	return t, nil
}

// ── Match Score Helper ──────────────────────────────────────────────────

// calculateMatchScore scores an editor against a list of required skills.
// Each matching skill = 20 points (up to 80), plus a rating bonus of
// avg_rating * 4 (up to 20). Total range: 0 -- 100.
func calculateMatchScore(jobSkills []string, editorSkills []model.EditorSkill, editorRating float64) int {
	if len(jobSkills) == 0 {
		// No skills to match; just use rating bonus
		ratingBonus := int(editorRating * 4)
		if ratingBonus > 20 {
			ratingBonus = 20
		}
		return ratingBonus
	}

	// Build set of editor skill names (lowercased)
	editorSet := make(map[string]bool, len(editorSkills))
	for _, s := range editorSkills {
		editorSet[strings.ToLower(s.SkillName)] = true
	}

	// Count matches
	matches := 0
	for _, js := range jobSkills {
		if editorSet[strings.ToLower(js)] {
			matches++
		}
	}

	// Skill score: 20 per match, capped at 80
	skillScore := matches * 20
	if skillScore > 80 {
		skillScore = 80
	}

	// Rating bonus: avg_rating * 4, capped at 20
	ratingBonus := int(editorRating * 4)
	if ratingBonus > 20 {
		ratingBonus = 20
	}

	total := skillScore + ratingBonus
	if total > 100 {
		total = 100
	}
	return total
}
