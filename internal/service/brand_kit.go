package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type BrandKitService struct {
	pool *pgxpool.Pool
}

func NewBrandKitService(pool *pgxpool.Pool) *BrandKitService {
	return &BrandKitService{pool: pool}
}

// ── Column lists & scanners ─────────────────────────────────────────────

const brandKitColumns = `id, user_id, name, COALESCE(logo_url,''), COALESCE(primary_color,''),
	COALESCE(secondary_color,''), COALESCE(accent_color,''), COALESCE(font_heading,''),
	COALESCE(font_body,''), tone, COALESCE(voice_description,''), COALESCE(tagline,''),
	COALESCE(hashtags,'{}'), COALESCE(cta_default,''), is_default, created_at, updated_at`

func scanBrandKit(row interface{ Scan(dest ...any) error }, k *model.BrandKit) error {
	return row.Scan(
		&k.ID, &k.UserID, &k.Name, &k.LogoURL, &k.PrimaryColor,
		&k.SecondaryColor, &k.AccentColor, &k.FontHeading,
		&k.FontBody, &k.Tone, &k.VoiceDescription, &k.Tagline,
		&k.Hashtags, &k.CTADefault, &k.IsDefault, &k.CreatedAt, &k.UpdatedAt,
	)
}

const templateColumns = `id, brand_kit_id, user_id, name, platform, template_body,
	COALESCE(variables,'{}'), COALESCE(example_output,''), category, is_active,
	created_at, updated_at`

func scanTemplate(row interface{ Scan(dest ...any) error }, t *model.ContentTemplate) error {
	return row.Scan(
		&t.ID, &t.BrandKitID, &t.UserID, &t.Name, &t.Platform,
		&t.TemplateBody, &t.Variables, &t.ExampleOutput, &t.Category,
		&t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
}

// ── Helpers ─────────────────────────────────────────────────────────────

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
var templateVarRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// validateHexColor returns true if c is a valid #RRGGBB hex color.
func validateHexColor(c string) bool {
	return hexColorRe.MatchString(c)
}

// extractTemplateVariables returns the unique variable names found in body.
func extractTemplateVariables(body string) []string {
	matches := templateVarRe.FindAllStringSubmatch(body, -1)
	seen := make(map[string]struct{})
	var vars []string
	for _, m := range matches {
		name := m[1]
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			vars = append(vars, name)
		}
	}
	return vars
}

// renderTemplateBody replaces every {{var}} in body with the corresponding
// value from vars. Returns an error if a required variable is missing.
func renderTemplateBody(body string, vars map[string]string) (string, error) {
	required := extractTemplateVariables(body)
	for _, v := range required {
		if _, ok := vars[v]; !ok {
			return "", fmt.Errorf("missing required variable: %s", v)
		}
	}
	result := templateVarRe.ReplaceAllStringFunc(body, func(match string) string {
		name := match[2 : len(match)-2] // strip {{ and }}
		return vars[name]
	})
	return result, nil
}

// validateColors validates any non-empty colour fields in the request.
func validateColors(colors ...string) error {
	for _, c := range colors {
		if c != "" && !validateHexColor(c) {
			return fmt.Errorf("invalid hex color: %s (expected #RRGGBB)", c)
		}
	}
	return nil
}

// bkNilIfEmpty returns nil for empty strings so the DB stores NULL.
func bkNilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── Brand Kit CRUD ──────────────────────────────────────────────────────

func (s *BrandKitService) CreateKit(ctx context.Context, userID uuid.UUID, req *model.CreateBrandKitRequest) (*model.BrandKit, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := validateColors(req.PrimaryColor, req.SecondaryColor, req.AccentColor); err != nil {
		return nil, err
	}

	tone := req.Tone
	if tone == "" {
		tone = "professional"
	}
	hashtags := req.Hashtags
	if hashtags == nil {
		hashtags = []string{}
	}

	var k model.BrandKit
	err := s.pool.QueryRow(ctx,
		`INSERT INTO brand_kits
			(user_id, name, logo_url, primary_color, secondary_color, accent_color,
			 font_heading, font_body, tone, voice_description, tagline, hashtags, cta_default)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING `+brandKitColumns,
		userID, req.Name, bkNilIfEmpty(req.LogoURL),
		bkNilIfEmpty(req.PrimaryColor), bkNilIfEmpty(req.SecondaryColor), bkNilIfEmpty(req.AccentColor),
		bkNilIfEmpty(req.FontHeading), bkNilIfEmpty(req.FontBody),
		tone, bkNilIfEmpty(req.VoiceDescription), bkNilIfEmpty(req.Tagline),
		hashtags, bkNilIfEmpty(req.CTADefault),
	).Scan(
		&k.ID, &k.UserID, &k.Name, &k.LogoURL, &k.PrimaryColor,
		&k.SecondaryColor, &k.AccentColor, &k.FontHeading,
		&k.FontBody, &k.Tone, &k.VoiceDescription, &k.Tagline,
		&k.Hashtags, &k.CTADefault, &k.IsDefault, &k.CreatedAt, &k.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("brand kit with name %q already exists", req.Name)
		}
		return nil, fmt.Errorf("create brand kit: %w", err)
	}
	return &k, nil
}

func (s *BrandKitService) GetKit(ctx context.Context, userID, kitID uuid.UUID) (*model.BrandKit, error) {
	var k model.BrandKit
	err := s.pool.QueryRow(ctx,
		`SELECT `+brandKitColumns+` FROM brand_kits WHERE id = $1 AND user_id = $2`,
		kitID, userID,
	).Scan(
		&k.ID, &k.UserID, &k.Name, &k.LogoURL, &k.PrimaryColor,
		&k.SecondaryColor, &k.AccentColor, &k.FontHeading,
		&k.FontBody, &k.Tone, &k.VoiceDescription, &k.Tagline,
		&k.Hashtags, &k.CTADefault, &k.IsDefault, &k.CreatedAt, &k.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("brand kit not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get brand kit: %w", err)
	}
	return &k, nil
}

func (s *BrandKitService) ListKits(ctx context.Context, userID uuid.UUID) ([]model.BrandKit, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+brandKitColumns+` FROM brand_kits WHERE user_id = $1 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list brand kits: %w", err)
	}
	defer rows.Close()

	var kits []model.BrandKit
	for rows.Next() {
		var k model.BrandKit
		if err := scanBrandKit(rows, &k); err != nil {
			return nil, fmt.Errorf("scan brand kit: %w", err)
		}
		kits = append(kits, k)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate brand kits: %w", err)
	}
	if kits == nil {
		kits = []model.BrandKit{}
	}
	return kits, nil
}

func (s *BrandKitService) UpdateKit(ctx context.Context, userID, kitID uuid.UUID, req *model.UpdateBrandKitRequest) (*model.BrandKit, error) {
	// Verify ownership
	_, err := s.GetKit(ctx, userID, kitID)
	if err != nil {
		return nil, err
	}

	var sets []string
	var args []interface{}
	argIdx := 1

	if req.Name != nil {
		if *req.Name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.LogoURL != nil {
		sets = append(sets, fmt.Sprintf("logo_url = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.LogoURL))
		argIdx++
	}
	if req.PrimaryColor != nil {
		if *req.PrimaryColor != "" && !validateHexColor(*req.PrimaryColor) {
			return nil, fmt.Errorf("invalid hex color: %s", *req.PrimaryColor)
		}
		sets = append(sets, fmt.Sprintf("primary_color = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.PrimaryColor))
		argIdx++
	}
	if req.SecondaryColor != nil {
		if *req.SecondaryColor != "" && !validateHexColor(*req.SecondaryColor) {
			return nil, fmt.Errorf("invalid hex color: %s", *req.SecondaryColor)
		}
		sets = append(sets, fmt.Sprintf("secondary_color = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.SecondaryColor))
		argIdx++
	}
	if req.AccentColor != nil {
		if *req.AccentColor != "" && !validateHexColor(*req.AccentColor) {
			return nil, fmt.Errorf("invalid hex color: %s", *req.AccentColor)
		}
		sets = append(sets, fmt.Sprintf("accent_color = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.AccentColor))
		argIdx++
	}
	if req.FontHeading != nil {
		sets = append(sets, fmt.Sprintf("font_heading = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.FontHeading))
		argIdx++
	}
	if req.FontBody != nil {
		sets = append(sets, fmt.Sprintf("font_body = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.FontBody))
		argIdx++
	}
	if req.Tone != nil {
		sets = append(sets, fmt.Sprintf("tone = $%d", argIdx))
		args = append(args, *req.Tone)
		argIdx++
	}
	if req.VoiceDescription != nil {
		sets = append(sets, fmt.Sprintf("voice_description = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.VoiceDescription))
		argIdx++
	}
	if req.Tagline != nil {
		sets = append(sets, fmt.Sprintf("tagline = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.Tagline))
		argIdx++
	}
	if req.Hashtags != nil {
		sets = append(sets, fmt.Sprintf("hashtags = $%d", argIdx))
		args = append(args, *req.Hashtags)
		argIdx++
	}
	if req.CTADefault != nil {
		sets = append(sets, fmt.Sprintf("cta_default = $%d", argIdx))
		args = append(args, bkNilIfEmpty(*req.CTADefault))
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetKit(ctx, userID, kitID)
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, kitID, userID)

	query := fmt.Sprintf(
		`UPDATE brand_kits SET %s WHERE id = $%d AND user_id = $%d
		 RETURNING `+brandKitColumns,
		strings.Join(sets, ", "), argIdx, argIdx+1,
	)

	var k model.BrandKit
	err = s.pool.QueryRow(ctx, query, args...).Scan(
		&k.ID, &k.UserID, &k.Name, &k.LogoURL, &k.PrimaryColor,
		&k.SecondaryColor, &k.AccentColor, &k.FontHeading,
		&k.FontBody, &k.Tone, &k.VoiceDescription, &k.Tagline,
		&k.Hashtags, &k.CTADefault, &k.IsDefault, &k.CreatedAt, &k.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("brand kit not found")
	}
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("brand kit with that name already exists")
		}
		return nil, fmt.Errorf("update brand kit: %w", err)
	}
	return &k, nil
}

func (s *BrandKitService) DeleteKit(ctx context.Context, userID, kitID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM brand_kits WHERE id = $1 AND user_id = $2`, kitID, userID)
	if err != nil {
		return fmt.Errorf("delete brand kit: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("brand kit not found")
	}
	return nil
}

func (s *BrandKitService) SetDefault(ctx context.Context, userID, kitID uuid.UUID) error {
	// Verify ownership
	_, err := s.GetKit(ctx, userID, kitID)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Unset all defaults for this user
	_, err = tx.Exec(ctx,
		`UPDATE brand_kits SET is_default = FALSE, updated_at = NOW() WHERE user_id = $1 AND is_default = TRUE`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("unset defaults: %w", err)
	}

	// Set the chosen kit as default
	_, err = tx.Exec(ctx,
		`UPDATE brand_kits SET is_default = TRUE, updated_at = NOW() WHERE id = $1 AND user_id = $2`,
		kitID, userID,
	)
	if err != nil {
		return fmt.Errorf("set default: %w", err)
	}

	return tx.Commit(ctx)
}

// ── Content Template CRUD ───────────────────────────────────────────────

func (s *BrandKitService) CreateTemplate(ctx context.Context, userID uuid.UUID, req *model.CreateTemplateRequest) (*model.ContentTemplate, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Platform == "" {
		return nil, fmt.Errorf("platform is required")
	}
	if req.TemplateBody == "" {
		return nil, fmt.Errorf("template_body is required")
	}

	// Verify brand kit ownership
	_, err := s.GetKit(ctx, userID, req.BrandKitID)
	if err != nil {
		return nil, fmt.Errorf("brand kit not found or not owned by user")
	}

	// Extract variables from body and validate against declared variables
	extracted := extractTemplateVariables(req.TemplateBody)
	variables := req.Variables
	if len(variables) == 0 {
		variables = extracted
	}
	if variables == nil {
		variables = []string{}
	}

	category := req.Category
	if category == "" {
		category = "general"
	}

	var t model.ContentTemplate
	err = s.pool.QueryRow(ctx,
		`INSERT INTO content_templates
			(brand_kit_id, user_id, name, platform, template_body, variables, example_output, category)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING `+templateColumns,
		req.BrandKitID, userID, req.Name, req.Platform, req.TemplateBody,
		variables, bkNilIfEmpty(req.ExampleOutput), category,
	).Scan(
		&t.ID, &t.BrandKitID, &t.UserID, &t.Name, &t.Platform,
		&t.TemplateBody, &t.Variables, &t.ExampleOutput, &t.Category,
		&t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}
	return &t, nil
}

func (s *BrandKitService) GetTemplate(ctx context.Context, userID, templateID uuid.UUID) (*model.ContentTemplate, error) {
	var t model.ContentTemplate
	err := s.pool.QueryRow(ctx,
		`SELECT `+templateColumns+` FROM content_templates WHERE id = $1 AND user_id = $2`,
		templateID, userID,
	).Scan(
		&t.ID, &t.BrandKitID, &t.UserID, &t.Name, &t.Platform,
		&t.TemplateBody, &t.Variables, &t.ExampleOutput, &t.Category,
		&t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("template not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}
	return &t, nil
}

func (s *BrandKitService) ListTemplates(ctx context.Context, userID uuid.UUID, brandKitID *uuid.UUID, platform string) ([]model.ContentTemplate, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
	args = append(args, userID)
	argIdx++

	if brandKitID != nil {
		conditions = append(conditions, fmt.Sprintf("brand_kit_id = $%d", argIdx))
		args = append(args, *brandKitID)
		argIdx++
	}
	if platform != "" {
		conditions = append(conditions, fmt.Sprintf("platform = $%d", argIdx))
		args = append(args, platform)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")
	query := `SELECT ` + templateColumns + ` FROM content_templates WHERE ` + where + ` ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var templates []model.ContentTemplate
	for rows.Next() {
		var t model.ContentTemplate
		if err := scanTemplate(rows, &t); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate templates: %w", err)
	}
	if templates == nil {
		templates = []model.ContentTemplate{}
	}
	return templates, nil
}

func (s *BrandKitService) RenderTemplate(ctx context.Context, userID uuid.UUID, req *model.RenderTemplateRequest) (*model.RenderedTemplate, error) {
	tmpl, err := s.GetTemplate(ctx, userID, req.TemplateID)
	if err != nil {
		return nil, err
	}

	rendered, err := renderTemplateBody(tmpl.TemplateBody, req.Variables)
	if err != nil {
		return nil, err
	}

	charCount := len([]rune(rendered))
	charLimit := model.PlatformCharLimits[tmpl.Platform]

	return &model.RenderedTemplate{
		Platform:     tmpl.Platform,
		RenderedText: rendered,
		CharCount:    charCount,
		CharLimit:    charLimit,
		IsOverLimit:  charLimit > 0 && charCount > charLimit,
	}, nil
}

func (s *BrandKitService) DeleteTemplate(ctx context.Context, userID, templateID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM content_templates WHERE id = $1 AND user_id = $2`, templateID, userID)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("template not found")
	}
	return nil
}
