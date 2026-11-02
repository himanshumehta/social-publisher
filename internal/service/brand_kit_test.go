package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── Helper validation tests (pure functions, no DB) ─────────────────────

func TestValidateHexColor_Valid(t *testing.T) {
	valid := []string{"#FF0000", "#00ff00", "#0a1B2c", "#000000", "#FFFFFF"}
	for _, c := range valid {
		if !validateHexColor(c) {
			t.Errorf("expected %q to be valid", c)
		}
	}
}

func TestValidateHexColor_Invalid(t *testing.T) {
	invalid := []string{
		"FF0000",   // missing #
		"#FF00",    // too short
		"#FF00000", // too long
		"#GGHHII",  // non-hex chars
		"",         // empty
		"#12345",   // 5 chars
		"red",      // named colour
	}
	for _, c := range invalid {
		if validateHexColor(c) {
			t.Errorf("expected %q to be invalid", c)
		}
	}
}

func TestExtractTemplateVariables(t *testing.T) {
	body := "Hey {{name}}, check out {{product}}! {{name}} loves it. #{{hashtag}}"
	vars := extractTemplateVariables(body)

	expected := map[string]bool{"name": true, "product": true, "hashtag": true}
	if len(vars) != len(expected) {
		t.Fatalf("expected %d unique vars, got %d: %v", len(expected), len(vars), vars)
	}
	for _, v := range vars {
		if !expected[v] {
			t.Errorf("unexpected variable: %s", v)
		}
	}
}

func TestExtractTemplateVariables_Empty(t *testing.T) {
	vars := extractTemplateVariables("no variables here")
	if len(vars) != 0 {
		t.Errorf("expected 0 vars, got %d: %v", len(vars), vars)
	}
}

func TestRenderTemplateBody_AllVarsProvided(t *testing.T) {
	body := "Hello {{name}}, welcome to {{platform}}!"
	vars := map[string]string{"name": "Alice", "platform": "Acme"}

	result, err := renderTemplateBody(body, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Hello Alice, welcome to Acme!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRenderTemplateBody_MissingVar(t *testing.T) {
	body := "Hello {{name}}, welcome to {{platform}}!"
	vars := map[string]string{"name": "Alice"} // missing "platform"

	_, err := renderTemplateBody(body, vars)
	if err == nil {
		t.Fatal("expected error for missing variable")
	}
	if !contains(err.Error(), "missing required variable") {
		t.Errorf("expected 'missing required variable' error, got: %v", err)
	}
}

func TestRenderTemplateBody_ExtraVarsIgnored(t *testing.T) {
	body := "Hello {{name}}!"
	vars := map[string]string{"name": "Alice", "extra": "ignored", "another": "also ignored"}

	result, err := renderTemplateBody(body, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello Alice!" {
		t.Errorf("expected 'Hello Alice!', got %q", result)
	}
}

func TestRenderTemplateBody_RepeatedVar(t *testing.T) {
	body := "{{name}} is great. {{name}} is awesome."
	vars := map[string]string{"name": "Acme"}

	result, err := renderTemplateBody(body, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Acme is great. Acme is awesome."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// ── Integration tests (require DB) ──────────────────────────────────────

func TestCreateKit_ValidatesColors(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")

	svc := NewBrandKitService(pool)

	// Valid colours should succeed
	kit, err := svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{
		Name:         "Test Kit " + uuid.New().String()[:8],
		PrimaryColor: "#FF0000",
		AccentColor:  "#00FF00",
	})
	if err != nil {
		t.Fatalf("expected success for valid colors, got: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM brand_kits WHERE id = $1", kit.ID)
	}()

	// Invalid colour should fail
	_, err = svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{
		Name:         "Bad Color Kit " + uuid.New().String()[:8],
		PrimaryColor: "not-a-color",
	})
	if err == nil {
		t.Error("expected error for invalid hex color")
	}
}

func TestCreateKit_DuplicateName(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")
	svc := NewBrandKitService(pool)

	name := "Duplicate Test " + uuid.New().String()[:8]

	kit, err := svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{Name: name})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM brand_kits WHERE id = $1", kit.ID)
	}()

	_, err = svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{Name: name})
	if err == nil {
		t.Error("expected error for duplicate brand kit name")
	}
}

func TestSetDefault_UnsetsOthers(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")
	svc := NewBrandKitService(pool)

	kit1, err := svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{
		Name: "Default A " + uuid.New().String()[:8],
	})
	if err != nil {
		t.Fatalf("create kit1 failed: %v", err)
	}
	kit2, err := svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{
		Name: "Default B " + uuid.New().String()[:8],
	})
	if err != nil {
		t.Fatalf("create kit2 failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM brand_kits WHERE id IN ($1, $2)", kit1.ID, kit2.ID)
	}()

	// Set kit1 as default
	if err := svc.SetDefault(ctx, userID, kit1.ID); err != nil {
		t.Fatalf("set default kit1 failed: %v", err)
	}
	k1, _ := svc.GetKit(ctx, userID, kit1.ID)
	if !k1.IsDefault {
		t.Error("expected kit1 to be default")
	}

	// Set kit2 as default -- kit1 should lose default
	if err := svc.SetDefault(ctx, userID, kit2.ID); err != nil {
		t.Fatalf("set default kit2 failed: %v", err)
	}
	k1, _ = svc.GetKit(ctx, userID, kit1.ID)
	k2, _ := svc.GetKit(ctx, userID, kit2.ID)
	if k1.IsDefault {
		t.Error("expected kit1 to no longer be default")
	}
	if !k2.IsDefault {
		t.Error("expected kit2 to be default")
	}
}

func TestCreateTemplate_ValidatesVariables(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")
	svc := NewBrandKitService(pool)

	kit, err := svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{
		Name: "Template Vars Kit " + uuid.New().String()[:8],
	})
	if err != nil {
		t.Fatalf("create kit failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM brand_kits WHERE id = $1", kit.ID)
	}()

	tmpl, err := svc.CreateTemplate(ctx, userID, &model.CreateTemplateRequest{
		BrandKitID:   kit.ID,
		Name:         "Promo " + uuid.New().String()[:8],
		Platform:     "twitter",
		TemplateBody: "Check out {{product}} by {{brand}}! #{{hashtag}}",
	})
	if err != nil {
		t.Fatalf("create template failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM content_templates WHERE id = $1", tmpl.ID)
	}()

	// Variables should have been auto-extracted
	if len(tmpl.Variables) != 3 {
		t.Errorf("expected 3 variables, got %d: %v", len(tmpl.Variables), tmpl.Variables)
	}
}

func TestRenderTemplate_ChecksCharLimit(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")
	svc := NewBrandKitService(pool)

	kit, err := svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{
		Name: "Render Kit " + uuid.New().String()[:8],
	})
	if err != nil {
		t.Fatalf("create kit failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM brand_kits WHERE id = $1", kit.ID)
	}()

	tmpl, err := svc.CreateTemplate(ctx, userID, &model.CreateTemplateRequest{
		BrandKitID:   kit.ID,
		Name:         "Short Tweet " + uuid.New().String()[:8],
		Platform:     "twitter",
		TemplateBody: "{{msg}}",
	})
	if err != nil {
		t.Fatalf("create template failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM content_templates WHERE id = $1", tmpl.ID)
	}()

	// Short message -- should be under limit
	result, err := svc.RenderTemplate(ctx, userID, &model.RenderTemplateRequest{
		TemplateID: tmpl.ID,
		Variables:  map[string]string{"msg": "Hello world!"},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if result.IsOverLimit {
		t.Error("expected short message to be within limit")
	}
	if result.CharLimit != 280 {
		t.Errorf("expected twitter char limit 280, got %d", result.CharLimit)
	}
	if result.RenderedText != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", result.RenderedText)
	}
}

func TestRenderTemplate_OverLimit(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")
	svc := NewBrandKitService(pool)

	kit, err := svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{
		Name: "Over Limit Kit " + uuid.New().String()[:8],
	})
	if err != nil {
		t.Fatalf("create kit failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM brand_kits WHERE id = $1", kit.ID)
	}()

	tmpl, err := svc.CreateTemplate(ctx, userID, &model.CreateTemplateRequest{
		BrandKitID:   kit.ID,
		Name:         "Long Tweet " + uuid.New().String()[:8],
		Platform:     "twitter",
		TemplateBody: "{{msg}}",
	})
	if err != nil {
		t.Fatalf("create template failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM content_templates WHERE id = $1", tmpl.ID)
	}()

	// Build a message that exceeds 280 chars
	longMsg := ""
	for i := 0; i < 300; i++ {
		longMsg += "A"
	}

	result, err := svc.RenderTemplate(ctx, userID, &model.RenderTemplateRequest{
		TemplateID: tmpl.ID,
		Variables:  map[string]string{"msg": longMsg},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !result.IsOverLimit {
		t.Error("expected 300-char message to exceed twitter limit of 280")
	}
	if result.CharCount != 300 {
		t.Errorf("expected char_count 300, got %d", result.CharCount)
	}
}

// ── Full CRUD lifecycle ─────────────────────────────────────────────────

func TestBrandKit_FullLifecycle(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")
	svc := NewBrandKitService(pool)

	// 1. Create
	kitName := "Lifecycle Kit " + uuid.New().String()[:8]
	kit, err := svc.CreateKit(ctx, userID, &model.CreateBrandKitRequest{
		Name:         kitName,
		PrimaryColor: "#112233",
		Tone:         "casual",
		Hashtags:     []string{"#acme", "#content"},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// 2. Get
	fetched, err := svc.GetKit(ctx, userID, kit.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if fetched.Name != kitName {
		t.Errorf("expected name %q, got %q", kitName, fetched.Name)
	}
	if fetched.PrimaryColor != "#112233" {
		t.Errorf("expected primary_color #112233, got %q", fetched.PrimaryColor)
	}
	if fetched.Tone != "casual" {
		t.Errorf("expected tone casual, got %q", fetched.Tone)
	}

	// 3. List
	kits, err := svc.ListKits(ctx, userID)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	found := false
	for _, k := range kits {
		if k.ID == kit.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected created kit in list results")
	}

	// 4. Update
	newTone := "bold"
	updated, err := svc.UpdateKit(ctx, userID, kit.ID, &model.UpdateBrandKitRequest{
		Tone: &newTone,
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Tone != "bold" {
		t.Errorf("expected tone bold after update, got %q", updated.Tone)
	}

	// 5. Create template under this kit
	tmpl, err := svc.CreateTemplate(ctx, userID, &model.CreateTemplateRequest{
		BrandKitID:   kit.ID,
		Name:         "Promo Template " + uuid.New().String()[:8],
		Platform:     "instagram",
		TemplateBody: "{{headline}}\n\n{{body}}\n\n{{cta}}",
	})
	if err != nil {
		t.Fatalf("create template failed: %v", err)
	}
	if tmpl.Platform != "instagram" {
		t.Errorf("expected platform instagram, got %q", tmpl.Platform)
	}

	// 6. Render template
	rendered, err := svc.RenderTemplate(ctx, userID, &model.RenderTemplateRequest{
		TemplateID: tmpl.ID,
		Variables: map[string]string{
			"headline": "Big Sale!",
			"body":     "50% off everything",
			"cta":      "Shop now at example.com",
		},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if rendered.Platform != "instagram" {
		t.Errorf("expected platform instagram, got %q", rendered.Platform)
	}
	if rendered.CharLimit != 2200 {
		t.Errorf("expected instagram char limit 2200, got %d", rendered.CharLimit)
	}
	if rendered.IsOverLimit {
		t.Error("expected rendered content to be within limit")
	}

	// 7. List templates
	templates, err := svc.ListTemplates(ctx, userID, &kit.ID, "")
	if err != nil {
		t.Fatalf("list templates failed: %v", err)
	}
	if len(templates) < 1 {
		t.Error("expected at least 1 template in list")
	}

	// 8. Delete template
	if err := svc.DeleteTemplate(ctx, userID, tmpl.ID); err != nil {
		t.Fatalf("delete template failed: %v", err)
	}
	_, err = svc.GetTemplate(ctx, userID, tmpl.ID)
	if err == nil {
		t.Error("expected error fetching deleted template")
	}

	// 9. Delete kit (should cascade-delete any remaining templates)
	if err := svc.DeleteKit(ctx, userID, kit.ID); err != nil {
		t.Fatalf("delete kit failed: %v", err)
	}
	_, err = svc.GetKit(ctx, userID, kit.ID)
	if err == nil {
		t.Error("expected error fetching deleted brand kit")
	}
}
