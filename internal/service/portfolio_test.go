package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── calculateMatchScore unit tests ──────────────────────────────────────

func TestCalculateMatchScore_FullMatch(t *testing.T) {
	jobSkills := []string{"editing", "color-grading", "motion-graphics", "sound-design"}
	editorSkills := []model.EditorSkill{
		{SkillName: "editing"},
		{SkillName: "color-grading"},
		{SkillName: "motion-graphics"},
		{SkillName: "sound-design"},
	}
	rating := 5.0

	score := calculateMatchScore(jobSkills, editorSkills, rating)

	// 4 matches * 20 = 80 skill points + 5.0 * 4 = 20 rating bonus = 100
	if score != 100 {
		t.Errorf("score = %d, want 100", score)
	}
}

func TestCalculateMatchScore_PartialMatch(t *testing.T) {
	jobSkills := []string{"editing", "color-grading", "motion-graphics", "sound-design"}
	editorSkills := []model.EditorSkill{
		{SkillName: "editing"},
		{SkillName: "color-grading"},
	}
	rating := 3.0

	score := calculateMatchScore(jobSkills, editorSkills, rating)

	// 2 matches * 20 = 40 skill points + 3.0 * 4 = 12 rating bonus = 52
	if score != 52 {
		t.Errorf("score = %d, want 52", score)
	}
}

func TestCalculateMatchScore_NoMatch(t *testing.T) {
	jobSkills := []string{"editing", "color-grading"}
	editorSkills := []model.EditorSkill{
		{SkillName: "motion-graphics"},
		{SkillName: "sound-design"},
	}
	rating := 2.0

	score := calculateMatchScore(jobSkills, editorSkills, rating)

	// 0 matches * 20 = 0 skill points + 2.0 * 4 = 8 rating bonus = 8
	if score != 8 {
		t.Errorf("score = %d, want 8", score)
	}
}

func TestCalculateMatchScore_HighRatingBonus(t *testing.T) {
	jobSkills := []string{"editing"}
	editorSkills := []model.EditorSkill{
		{SkillName: "editing"},
	}
	rating := 5.0

	score := calculateMatchScore(jobSkills, editorSkills, rating)

	// 1 match * 20 = 20 skill points + 5.0 * 4 = 20 rating bonus = 40
	if score != 40 {
		t.Errorf("score = %d, want 40", score)
	}
}

func TestCalculateMatchScore_EmptyJobSkills(t *testing.T) {
	jobSkills := []string{}
	editorSkills := []model.EditorSkill{
		{SkillName: "editing"},
	}
	rating := 4.5

	score := calculateMatchScore(jobSkills, editorSkills, rating)

	// No job skills -> only rating bonus = 4.5 * 4 = 18
	if score != 18 {
		t.Errorf("score = %d, want 18", score)
	}
}

func TestCalculateMatchScore_CaseInsensitive(t *testing.T) {
	jobSkills := []string{"Editing", "COLOR-GRADING"}
	editorSkills := []model.EditorSkill{
		{SkillName: "editing"},
		{SkillName: "color-grading"},
	}
	rating := 0.0

	score := calculateMatchScore(jobSkills, editorSkills, rating)

	// 2 matches * 20 = 40 skill points + 0 rating bonus = 40
	if score != 40 {
		t.Errorf("score = %d, want 40", score)
	}
}

func TestCalculateMatchScore_SkillCapAt80(t *testing.T) {
	// 5 matching skills should cap at 80 (not 100)
	jobSkills := []string{"a", "b", "c", "d", "e"}
	editorSkills := []model.EditorSkill{
		{SkillName: "a"},
		{SkillName: "b"},
		{SkillName: "c"},
		{SkillName: "d"},
		{SkillName: "e"},
	}
	rating := 0.0

	score := calculateMatchScore(jobSkills, editorSkills, rating)

	// 5 matches * 20 = 100 -> capped at 80 + 0 rating bonus = 80
	if score != 80 {
		t.Errorf("score = %d, want 80", score)
	}
}

func TestCalculateMatchScore_RatingCapAt20(t *testing.T) {
	// Edge case: rating above 5 should still cap bonus at 20
	jobSkills := []string{}
	editorSkills := []model.EditorSkill{}
	rating := 6.0 // hypothetical

	score := calculateMatchScore(jobSkills, editorSkills, rating)

	// 6.0 * 4 = 24 -> capped at 20
	if score != 20 {
		t.Errorf("score = %d, want 20", score)
	}
}

// ── Validation unit tests ───────────────────────────────────────────────

func TestValidAvailabilities_AllPresent(t *testing.T) {
	expected := []string{"available", "busy", "unavailable"}
	for _, a := range expected {
		if !validAvailabilities[a] {
			t.Errorf("validAvailabilities[%q] = false, want true", a)
		}
	}
	if len(validAvailabilities) != len(expected) {
		t.Errorf("len(validAvailabilities) = %d, want %d", len(validAvailabilities), len(expected))
	}
}

func TestValidProficiencies_AllPresent(t *testing.T) {
	expected := []string{"beginner", "intermediate", "advanced", "expert"}
	for _, p := range expected {
		if !validProficiencies[p] {
			t.Errorf("validProficiencies[%q] = false, want true", p)
		}
	}
	if len(validProficiencies) != len(expected) {
		t.Errorf("len(validProficiencies) = %d, want %d", len(validProficiencies), len(expected))
	}
}

func TestValidMediaTypes_AllPresent(t *testing.T) {
	expected := []string{"video", "image", "link"}
	for _, m := range expected {
		if !validMediaTypes[m] {
			t.Errorf("validMediaTypes[%q] = false, want true", m)
		}
	}
	if len(validMediaTypes) != len(expected) {
		t.Errorf("len(validMediaTypes) = %d, want %d", len(validMediaTypes), len(expected))
	}
}

// ── Integration test helpers ────────────────────────────────────────────

type portfolioTestState struct {
	ctx       context.Context
	editorID  uuid.UUID
	creatorID uuid.UUID
}

func setupPortfolioTest(t *testing.T) (svc *PortfolioService, st portfolioTestState, cleanup func()) {
	t.Helper()

	pool := setupTestPool(t)
	svc = NewPortfolioService(pool)

	ctx := context.Background()
	editorID := uuid.New()
	creatorID := uuid.New()

	// Insert editor user
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'editor')`,
		editorID, "Portfolio Editor", "ped-"+editorID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert editor user: %v", err)
	}

	// Insert creator user
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		creatorID, "Portfolio Creator", "pcr-"+creatorID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert creator user: %v", err)
	}

	st = portfolioTestState{
		ctx:       ctx,
		editorID:  editorID,
		creatorID: creatorID,
	}

	cleanup = func() {
		pool.Exec(ctx, `DELETE FROM portfolio_items WHERE portfolio_id IN (SELECT id FROM editor_portfolios WHERE editor_id = $1)`, editorID)
		pool.Exec(ctx, `DELETE FROM editor_portfolios WHERE editor_id = $1`, editorID)
		pool.Exec(ctx, `DELETE FROM editor_skills WHERE editor_id = $1`, editorID)
		pool.Exec(ctx, `DELETE FROM marketplace_templates WHERE creator_id = $1`, creatorID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, editorID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, creatorID)
		pool.Close()
	}

	return svc, st, cleanup
}

// ── Integration Tests ───────────────────────────────────────────────────

func TestCreatePortfolio_Success(t *testing.T) {
	svc, st, cleanup := setupPortfolioTest(t)
	defer cleanup()

	portfolio, err := svc.CreatePortfolio(st.ctx, st.editorID, &model.CreatePortfolioRequest{
		Bio:             "Experienced video editor specializing in YouTube content.",
		Specializations: []string{"youtube", "short-form"},
		YearsExperience: 5,
		HourlyRate:      250000, // 2500.00 INR in paise
		Availability:    "available",
		PortfolioURL:    "https://portfolio.example.com",
		DemoReelURL:     "https://youtube.com/demo",
	})
	if err != nil {
		t.Fatalf("CreatePortfolio failed: %v", err)
	}

	if portfolio.ID == uuid.Nil {
		t.Error("expected portfolio ID to be set")
	}
	if portfolio.EditorID != st.editorID {
		t.Errorf("editor_id = %s, want %s", portfolio.EditorID, st.editorID)
	}
	if portfolio.Bio != "Experienced video editor specializing in YouTube content." {
		t.Errorf("bio = %q, want %q", portfolio.Bio, "Experienced video editor specializing in YouTube content.")
	}
	if len(portfolio.Specializations) != 2 {
		t.Errorf("specializations len = %d, want 2", len(portfolio.Specializations))
	}
	if portfolio.YearsExperience != 5 {
		t.Errorf("years_experience = %d, want 5", portfolio.YearsExperience)
	}
	if portfolio.HourlyRate != 250000 {
		t.Errorf("hourly_rate = %d, want 250000", portfolio.HourlyRate)
	}
	if portfolio.Availability != model.AvailabilityAvailable {
		t.Errorf("availability = %q, want %q", portfolio.Availability, model.AvailabilityAvailable)
	}
	if portfolio.PortfolioURL != "https://portfolio.example.com" {
		t.Errorf("portfolio_url = %q, want %q", portfolio.PortfolioURL, "https://portfolio.example.com")
	}
	if portfolio.Skills == nil {
		t.Error("skills should not be nil")
	}
	if portfolio.Items == nil {
		t.Error("items should not be nil")
	}
}

func TestCreatePortfolio_AlreadyExists(t *testing.T) {
	svc, st, cleanup := setupPortfolioTest(t)
	defer cleanup()

	// Create first portfolio
	_, err := svc.CreatePortfolio(st.ctx, st.editorID, &model.CreatePortfolioRequest{
		Bio: "First portfolio",
	})
	if err != nil {
		t.Fatalf("first CreatePortfolio failed: %v", err)
	}

	// Attempt duplicate
	_, err = svc.CreatePortfolio(st.ctx, st.editorID, &model.CreatePortfolioRequest{
		Bio: "Second portfolio",
	})
	if err == nil {
		t.Fatal("expected error for duplicate portfolio, got nil")
	}
	if got := err.Error(); got != "portfolio already exists for this editor" {
		t.Errorf("error = %q, want %q", got, "portfolio already exists for this editor")
	}
}

func TestSetSkills_ReplacesExisting(t *testing.T) {
	svc, st, cleanup := setupPortfolioTest(t)
	defer cleanup()

	// Set initial skills
	skills1, err := svc.SetSkills(st.ctx, st.editorID, &model.SetSkillsRequest{
		Skills: []model.SkillInput{
			{Name: "editing", Proficiency: "advanced"},
			{Name: "color-grading", Proficiency: "intermediate"},
		},
	})
	if err != nil {
		t.Fatalf("first SetSkills failed: %v", err)
	}
	if len(skills1) != 2 {
		t.Fatalf("skills len = %d, want 2", len(skills1))
	}

	// Replace with new set
	skills2, err := svc.SetSkills(st.ctx, st.editorID, &model.SetSkillsRequest{
		Skills: []model.SkillInput{
			{Name: "motion-graphics", Proficiency: "expert"},
			{Name: "sound-design", Proficiency: "beginner"},
			{Name: "vfx", Proficiency: "intermediate"},
		},
	})
	if err != nil {
		t.Fatalf("second SetSkills failed: %v", err)
	}
	if len(skills2) != 3 {
		t.Errorf("skills len = %d, want 3", len(skills2))
	}

	// Verify old skills are gone by checking names
	nameSet := make(map[string]bool)
	for _, s := range skills2 {
		nameSet[s.SkillName] = true
	}
	if nameSet["editing"] {
		t.Error("old skill 'editing' should have been removed")
	}
	if nameSet["color-grading"] {
		t.Error("old skill 'color-grading' should have been removed")
	}
	if !nameSet["motion-graphics"] {
		t.Error("new skill 'motion-graphics' should be present")
	}
	if !nameSet["sound-design"] {
		t.Error("new skill 'sound-design' should be present")
	}
	if !nameSet["vfx"] {
		t.Error("new skill 'vfx' should be present")
	}
}

func TestSearchEditors_BySkill(t *testing.T) {
	svc, st, cleanup := setupPortfolioTest(t)
	defer cleanup()

	// Create portfolio and skills
	_, err := svc.CreatePortfolio(st.ctx, st.editorID, &model.CreatePortfolioRequest{
		Bio:          "Skilled editor",
		HourlyRate:   100000,
		Availability: "available",
	})
	if err != nil {
		t.Fatalf("CreatePortfolio failed: %v", err)
	}

	_, err = svc.SetSkills(st.ctx, st.editorID, &model.SetSkillsRequest{
		Skills: []model.SkillInput{
			{Name: "editing", Proficiency: "advanced"},
			{Name: "color-grading", Proficiency: "intermediate"},
		},
	})
	if err != nil {
		t.Fatalf("SetSkills failed: %v", err)
	}

	// Search by matching skill
	results, total, err := svc.SearchEditors(st.ctx, &model.EditorSearchFilter{
		Skills:   []string{"editing"},
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("SearchEditors failed: %v", err)
	}

	if total < 1 {
		t.Errorf("total = %d, want >= 1", total)
	}

	found := false
	for _, r := range results {
		if r.EditorID == st.editorID {
			found = true
			if r.MatchScore == 0 {
				t.Error("expected non-zero match score for matching editor")
			}
			break
		}
	}
	if !found {
		t.Error("expected test editor in search results")
	}
}

func TestSearchEditors_ByAvailability(t *testing.T) {
	svc, st, cleanup := setupPortfolioTest(t)
	defer cleanup()

	// Create portfolio with "busy" status
	_, err := svc.CreatePortfolio(st.ctx, st.editorID, &model.CreatePortfolioRequest{
		Bio:          "Busy editor",
		Availability: "busy",
	})
	if err != nil {
		t.Fatalf("CreatePortfolio failed: %v", err)
	}

	// Search for available editors should NOT return this editor
	results, _, err := svc.SearchEditors(st.ctx, &model.EditorSearchFilter{
		Availability: "available",
		Page:         1,
		PageSize:     20,
	})
	if err != nil {
		t.Fatalf("SearchEditors failed: %v", err)
	}

	for _, r := range results {
		if r.EditorID == st.editorID {
			t.Error("busy editor should not appear in 'available' search")
		}
	}

	// Search for busy editors SHOULD return this editor
	results2, _, err := svc.SearchEditors(st.ctx, &model.EditorSearchFilter{
		Availability: "busy",
		Page:         1,
		PageSize:     20,
	})
	if err != nil {
		t.Fatalf("SearchEditors (busy) failed: %v", err)
	}

	found := false
	for _, r := range results2 {
		if r.EditorID == st.editorID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected busy editor in 'busy' search results")
	}
}

func TestSearchEditors_Pagination(t *testing.T) {
	svc, st, cleanup := setupPortfolioTest(t)
	defer cleanup()

	// Create portfolio
	_, err := svc.CreatePortfolio(st.ctx, st.editorID, &model.CreatePortfolioRequest{
		Bio:          "Paginated editor",
		Availability: "available",
	})
	if err != nil {
		t.Fatalf("CreatePortfolio failed: %v", err)
	}

	// Page 1 with page_size=1 should return at most 1 result
	results, total, err := svc.SearchEditors(st.ctx, &model.EditorSearchFilter{
		Page:     1,
		PageSize: 1,
	})
	if err != nil {
		t.Fatalf("SearchEditors failed: %v", err)
	}

	if len(results) > 1 {
		t.Errorf("results len = %d, want <= 1 for page_size=1", len(results))
	}

	// Total should reflect all matching editors, not just page
	if total < 1 {
		t.Errorf("total = %d, want >= 1", total)
	}
}

func TestPurchaseTemplate_IncrementsCount(t *testing.T) {
	svc, st, cleanup := setupPortfolioTest(t)
	defer cleanup()

	// Create a template
	tmpl, err := svc.CreateMarketplaceTemplate(st.ctx, st.creatorID, &model.CreateMarketplaceTemplateRequest{
		Title:         "Cinematic Vlog Brief",
		Description:   "A template for cinematic vlog editing briefs.",
		Category:      "editing",
		BriefTemplate: "Please color grade with warm tones, add transitions...",
		Price:         50000, // 500.00 INR
	})
	if err != nil {
		t.Fatalf("CreateMarketplaceTemplate failed: %v", err)
	}

	if tmpl.PurchaseCount != 0 {
		t.Errorf("initial purchase_count = %d, want 0", tmpl.PurchaseCount)
	}

	// Purchase once
	purchased, err := svc.PurchaseTemplate(st.ctx, st.editorID, tmpl.ID)
	if err != nil {
		t.Fatalf("PurchaseTemplate failed: %v", err)
	}
	if purchased.PurchaseCount != 1 {
		t.Errorf("purchase_count after first purchase = %d, want 1", purchased.PurchaseCount)
	}

	// Purchase again
	purchased2, err := svc.PurchaseTemplate(st.ctx, st.editorID, tmpl.ID)
	if err != nil {
		t.Fatalf("second PurchaseTemplate failed: %v", err)
	}
	if purchased2.PurchaseCount != 2 {
		t.Errorf("purchase_count after second purchase = %d, want 2", purchased2.PurchaseCount)
	}
}

func TestCreateMarketplaceTemplate_Validation(t *testing.T) {
	svc, st, cleanup := setupPortfolioTest(t)
	defer cleanup()

	// Missing title
	_, err := svc.CreateMarketplaceTemplate(st.ctx, st.creatorID, &model.CreateMarketplaceTemplateRequest{
		Category:      "editing",
		BriefTemplate: "Some content",
	})
	if err == nil {
		t.Error("expected error for missing title")
	}

	// Missing category
	_, err = svc.CreateMarketplaceTemplate(st.ctx, st.creatorID, &model.CreateMarketplaceTemplateRequest{
		Title:         "Good Title",
		BriefTemplate: "Some content",
	})
	if err == nil {
		t.Error("expected error for missing category")
	}

	// Missing brief_template
	_, err = svc.CreateMarketplaceTemplate(st.ctx, st.creatorID, &model.CreateMarketplaceTemplateRequest{
		Title:    "Good Title",
		Category: "editing",
	})
	if err == nil {
		t.Error("expected error for missing brief_template")
	}

	// Negative price
	_, err = svc.CreateMarketplaceTemplate(st.ctx, st.creatorID, &model.CreateMarketplaceTemplateRequest{
		Title:         "Good Title",
		Category:      "editing",
		BriefTemplate: "Content",
		Price:         -100,
	})
	if err == nil {
		t.Error("expected error for negative price")
	}
}
