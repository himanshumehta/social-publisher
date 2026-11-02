package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// strPtr returns a pointer to the given string, for use in struct literals.
func strPtr(s string) *string { return &s }

// ── Validation tests ──────────────────────────────────────────────────

func TestValidatePlatformIntegrationType_YouTubeSuperChat(t *testing.T) {
	if !validatePlatformIntegrationType("youtube", "super_chat") {
		t.Error("expected youtube/super_chat to be valid")
	}
}

func TestValidatePlatformIntegrationType_InstagramShopping(t *testing.T) {
	if !validatePlatformIntegrationType("instagram", "shopping") {
		t.Error("expected instagram/shopping to be valid")
	}
}

func TestValidatePlatformIntegrationType_Invalid(t *testing.T) {
	// youtube + shopping is not a valid combination
	if validatePlatformIntegrationType("youtube", "shopping") {
		t.Error("expected youtube/shopping to be invalid")
	}
}

func TestValidatePlatformIntegrationType_UnknownPlatform(t *testing.T) {
	if validatePlatformIntegrationType("tiktok", "super_chat") {
		t.Error("expected tiktok/super_chat to be invalid")
	}
}

func TestValidatePlatformIntegrationType_AllValidCombos(t *testing.T) {
	validCombos := []struct {
		platform string
		intType  string
	}{
		{"youtube", "super_chat"},
		{"youtube", "membership"},
		{"instagram", "shopping"},
		{"shopify", "shopping"},
		{"shopify", "digital_product"},
		{"gumroad", "digital_product"},
		{"patreon", "subscription"},
		{"patreon", "membership"},
	}

	for _, tc := range validCombos {
		if !validatePlatformIntegrationType(tc.platform, tc.intType) {
			t.Errorf("expected %s/%s to be valid", tc.platform, tc.intType)
		}
	}
}

// ── calculateTopProducts tests ────────────────────────────────────────

func TestCalculateTopProducts_SortsByRevenue(t *testing.T) {
	txns := []model.CommerceTransaction{
		{ProductName: strPtr("Course A"), AmountPaise: 100_00},
		{ProductName: strPtr("Course B"), AmountPaise: 500_00},
		{ProductName: strPtr("Course C"), AmountPaise: 300_00},
	}

	result := calculateTopProducts(txns, 10)

	if len(result) != 3 {
		t.Fatalf("expected 3 products, got %d", len(result))
	}
	if result[0].Name != "Course B" {
		t.Errorf("expected top product = Course B, got %s", result[0].Name)
	}
	if result[0].RevenuePaise != 500_00 {
		t.Errorf("expected revenue 50000, got %d", result[0].RevenuePaise)
	}
	if result[1].Name != "Course C" {
		t.Errorf("expected second product = Course C, got %s", result[1].Name)
	}
}

func TestCalculateTopProducts_LimitsResults(t *testing.T) {
	txns := []model.CommerceTransaction{
		{ProductName: strPtr("A"), AmountPaise: 100},
		{ProductName: strPtr("B"), AmountPaise: 200},
		{ProductName: strPtr("C"), AmountPaise: 300},
		{ProductName: strPtr("D"), AmountPaise: 400},
		{ProductName: strPtr("E"), AmountPaise: 500},
	}

	result := calculateTopProducts(txns, 3)

	if len(result) != 3 {
		t.Fatalf("expected 3 products, got %d", len(result))
	}
	if result[0].Name != "E" {
		t.Errorf("expected top = E, got %s", result[0].Name)
	}
}

func TestCalculateTopProducts_AggregatesSameProduct(t *testing.T) {
	txns := []model.CommerceTransaction{
		{ProductName: strPtr("Course A"), AmountPaise: 100_00},
		{ProductName: strPtr("Course A"), AmountPaise: 200_00},
		{ProductName: strPtr("Course B"), AmountPaise: 150_00},
		{ProductName: strPtr("Course A"), AmountPaise: 50_00},
	}

	result := calculateTopProducts(txns, 10)

	if len(result) != 2 {
		t.Fatalf("expected 2 products, got %d", len(result))
	}
	// Course A: 100+200+50 = 350, Course B: 150
	if result[0].Name != "Course A" {
		t.Errorf("expected top = Course A, got %s", result[0].Name)
	}
	if result[0].RevenuePaise != 350_00 {
		t.Errorf("expected revenue 35000, got %d", result[0].RevenuePaise)
	}
	if result[0].Count != 3 {
		t.Errorf("expected count 3, got %d", result[0].Count)
	}
}

func TestCalculateTopProducts_SkipsEmptyProductNames(t *testing.T) {
	txns := []model.CommerceTransaction{
		{ProductName: strPtr(""), AmountPaise: 100_00},
		{ProductName: strPtr("Course A"), AmountPaise: 200_00},
	}

	result := calculateTopProducts(txns, 10)

	if len(result) != 1 {
		t.Fatalf("expected 1 product, got %d", len(result))
	}
	if result[0].Name != "Course A" {
		t.Errorf("expected Course A, got %s", result[0].Name)
	}
}

func TestCalculateTopProducts_EmptyInput(t *testing.T) {
	result := calculateTopProducts(nil, 10)
	if len(result) != 0 {
		t.Errorf("expected 0 products, got %d", len(result))
	}
}

// ── Integration tests (require database) ──────────────────────────────

func TestConnectIntegration_DuplicatePrevented(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Commerce Test", "commerce-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM commerce_integrations WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	req := &model.ConnectCommerceRequest{
		Platform:        "youtube",
		IntegrationType: "super_chat",
		Credentials:     map[string]string{"access_token": "test-token"},
	}

	// First connect should succeed
	ci, err := svc.ConnectIntegration(ctx, userID, req)
	if err != nil {
		t.Fatalf("first connect failed: %v", err)
	}
	if ci.Platform != "youtube" {
		t.Errorf("expected platform youtube, got %s", ci.Platform)
	}
	if ci.Status != "active" {
		t.Errorf("expected status active, got %s", ci.Status)
	}

	// Duplicate should fail (unique constraint)
	_, err = svc.ConnectIntegration(ctx, userID, req)
	if err == nil {
		t.Error("expected error for duplicate integration, got nil")
	}
}

func TestListIntegrations_HidesCredentials(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Commerce Cred Test", "cred-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM commerce_integrations WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	req := &model.ConnectCommerceRequest{
		Platform:        "gumroad",
		IntegrationType: "digital_product",
		Credentials:     map[string]string{"access_token": "secret-gumroad-token"},
	}

	_, err = svc.ConnectIntegration(ctx, userID, req)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	list, err := svc.ListIntegrations(ctx, userID)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 integration, got %d", len(list))
	}
	// Credentials must be nil (never loaded by ListIntegrations)
	if list[0].Credentials != nil {
		t.Error("expected credentials to be nil in list response")
	}
}

func TestRecordTransaction_Upsert(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Upsert Test", "upsert-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	req := &model.ConnectCommerceRequest{
		Platform:        "shopify",
		IntegrationType: "shopping",
		Credentials:     map[string]string{"api_key": "test"},
	}
	ci, err := svc.ConnectIntegration(ctx, userID, req)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	defer pool.Exec(ctx, `DELETE FROM commerce_transactions WHERE integration_id = $1`, ci.ID)
	defer pool.Exec(ctx, `DELETE FROM commerce_integrations WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	txn := &model.CommerceTransaction{
		TransactionType:       "product_sale",
		ExternalTransactionID: strPtr("shopify-order-12345"),
		AmountPaise:           999_00,
		Currency:              "INR",
		CustomerName:          strPtr("Jane Doe"),
		ProductName:           strPtr("Premium Course"),
		TransactionDate:       time.Now(),
	}

	// First insert
	err = svc.RecordTransaction(ctx, ci.ID, txn)
	if err != nil {
		t.Fatalf("first record failed: %v", err)
	}

	// Upsert with updated amount
	txn.AmountPaise = 1099_00
	txn.CustomerName = strPtr("Jane Doe Updated")
	err = svc.RecordTransaction(ctx, ci.ID, txn)
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	// Verify only one row exists
	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM commerce_transactions
		 WHERE integration_id = $1 AND external_transaction_id = $2`,
		ci.ID, "shopify-order-12345",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 transaction after upsert, got %d", count)
	}

	// Verify updated values
	var amount int64
	var name string
	err = pool.QueryRow(ctx,
		`SELECT amount_paise, customer_name FROM commerce_transactions
		 WHERE integration_id = $1 AND external_transaction_id = $2`,
		ci.ID, "shopify-order-12345",
	).Scan(&amount, &name)
	if err != nil {
		t.Fatalf("verify query failed: %v", err)
	}
	if amount != 1099_00 {
		t.Errorf("expected amount 109900, got %d", amount)
	}
	if name != "Jane Doe Updated" {
		t.Errorf("expected customer_name 'Jane Doe Updated', got %q", name)
	}
}

func TestGetStats_AggregatesByPlatform(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Stats Test", "stats-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Create two integrations on different platforms
	ytCI, err := svc.ConnectIntegration(ctx, userID, &model.ConnectCommerceRequest{
		Platform:        "youtube",
		IntegrationType: "super_chat",
		Credentials:     map[string]string{"token": "yt"},
	})
	if err != nil {
		t.Fatalf("connect youtube: %v", err)
	}

	shopCI, err := svc.ConnectIntegration(ctx, userID, &model.ConnectCommerceRequest{
		Platform:        "shopify",
		IntegrationType: "shopping",
		Credentials:     map[string]string{"key": "shop"},
	})
	if err != nil {
		t.Fatalf("connect shopify: %v", err)
	}

	now := time.Now()

	defer pool.Exec(ctx, `DELETE FROM commerce_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM commerce_integrations WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// Insert transactions
	for i, txn := range []struct {
		intID   uuid.UUID
		txnType string
		amount  int64
		extID   string
		product *string
	}{
		{ytCI.ID, "super_chat", 500_00, "yt-sc-1", strPtr("")},
		{ytCI.ID, "super_chat", 300_00, "yt-sc-2", strPtr("")},
		{shopCI.ID, "product_sale", 2000_00, "shop-1", strPtr("Course A")},
		{shopCI.ID, "product_sale", 1500_00, "shop-2", strPtr("Course B")},
	} {
		err := svc.RecordTransaction(ctx, txn.intID, &model.CommerceTransaction{
			TransactionType:       txn.txnType,
			ExternalTransactionID: strPtr(txn.extID),
			AmountPaise:           txn.amount,
			Currency:              "INR",
			ProductName:           txn.product,
			TransactionDate:       now.Add(-time.Duration(i) * time.Hour),
		})
		if err != nil {
			t.Fatalf("record txn %d: %v", i, err)
		}
	}

	start := now.AddDate(0, 0, -1)
	end := now.AddDate(0, 0, 1)

	stats, err := svc.GetStats(ctx, userID, start, end)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}

	if stats.TotalRevenuePaise != 4300_00 {
		t.Errorf("expected total 430000 paise, got %d", stats.TotalRevenuePaise)
	}
	if stats.TransactionCount != 4 {
		t.Errorf("expected 4 transactions, got %d", stats.TransactionCount)
	}

	ytStats, ok := stats.ByPlatform["youtube"]
	if !ok {
		t.Fatal("expected youtube in ByPlatform")
	}
	if ytStats.RevenuePaise != 800_00 {
		t.Errorf("expected youtube revenue 80000, got %d", ytStats.RevenuePaise)
	}
	if ytStats.TransactionCount != 2 {
		t.Errorf("expected youtube count 2, got %d", ytStats.TransactionCount)
	}

	shopStats, ok := stats.ByPlatform["shopify"]
	if !ok {
		t.Fatal("expected shopify in ByPlatform")
	}
	if shopStats.RevenuePaise != 3500_00 {
		t.Errorf("expected shopify revenue 350000, got %d", shopStats.RevenuePaise)
	}
}

func TestGetStats_AggregatesByType(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Type Stats", "typestats-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ci, err := svc.ConnectIntegration(ctx, userID, &model.ConnectCommerceRequest{
		Platform:        "youtube",
		IntegrationType: "super_chat",
		Credentials:     map[string]string{"token": "t"},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	now := time.Now()
	defer pool.Exec(ctx, `DELETE FROM commerce_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM commerce_integrations WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	_ = svc.RecordTransaction(ctx, ci.ID, &model.CommerceTransaction{
		TransactionType:       "super_chat",
		ExternalTransactionID: strPtr("sc-1"),
		AmountPaise:           500_00,
		Currency:              "INR",
		TransactionDate:       now,
	})
	_ = svc.RecordTransaction(ctx, ci.ID, &model.CommerceTransaction{
		TransactionType:       "tip",
		ExternalTransactionID: strPtr("tip-1"),
		AmountPaise:           200_00,
		Currency:              "INR",
		TransactionDate:       now,
	})

	stats, err := svc.GetStats(ctx, userID, now.AddDate(0, 0, -1), now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}

	if stats.ByType["super_chat"] != 500_00 {
		t.Errorf("expected super_chat revenue 50000, got %d", stats.ByType["super_chat"])
	}
	if stats.ByType["tip"] != 200_00 {
		t.Errorf("expected tip revenue 20000, got %d", stats.ByType["tip"])
	}
}

// ── Integration Tests (Full Flow) ─────────────────────────────────────

func TestCommerceIntegration_FullFlow(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	// Setup: create user
	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Flow Test", "flow-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM commerce_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM commerce_integrations WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// 1. Connect integration
	ci, err := svc.ConnectIntegration(ctx, userID, &model.ConnectCommerceRequest{
		Platform:        "shopify",
		IntegrationType: "shopping",
		Credentials:     map[string]string{"api_key": "sk-test", "shop": "myshop"},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if ci.Status != "active" {
		t.Errorf("expected active, got %s", ci.Status)
	}

	// 2. Record transactions
	now := time.Now()
	txns := []model.CommerceTransaction{
		{
			TransactionType:       "product_sale",
			ExternalTransactionID: strPtr("order-001"),
			AmountPaise:           2500_00,
			Currency:              "INR",
			CustomerName:          strPtr("Alice"),
			ProductName:           strPtr("Pro Course"),
			TransactionDate:       now.Add(-2 * time.Hour),
		},
		{
			TransactionType:       "product_sale",
			ExternalTransactionID: strPtr("order-002"),
			AmountPaise:           1500_00,
			Currency:              "INR",
			CustomerName:          strPtr("Bob"),
			ProductName:           strPtr("Starter Kit"),
			TransactionDate:       now.Add(-1 * time.Hour),
		},
		{
			TransactionType:       "product_sale",
			ExternalTransactionID: strPtr("order-003"),
			AmountPaise:           2500_00,
			Currency:              "INR",
			CustomerName:          strPtr("Charlie"),
			ProductName:           strPtr("Pro Course"),
			TransactionDate:       now,
		},
	}
	for i, txn := range txns {
		if err := svc.RecordTransaction(ctx, ci.ID, &txn); err != nil {
			t.Fatalf("record txn %d: %v", i, err)
		}
	}

	// 3. Get stats
	stats, err := svc.GetStats(ctx, userID, now.AddDate(0, 0, -1), now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.TotalRevenuePaise != 6500_00 {
		t.Errorf("expected total 650000 paise, got %d", stats.TotalRevenuePaise)
	}
	if stats.TransactionCount != 3 {
		t.Errorf("expected 3 transactions, got %d", stats.TransactionCount)
	}
	if len(stats.TopProducts) < 2 {
		t.Fatalf("expected at least 2 top products, got %d", len(stats.TopProducts))
	}
	// Pro Course: 2500+2500 = 5000, should be first
	if stats.TopProducts[0].Name != "Pro Course" {
		t.Errorf("expected top product = Pro Course, got %s", stats.TopProducts[0].Name)
	}
	if stats.TopProducts[0].RevenuePaise != 5000_00 {
		t.Errorf("expected Pro Course revenue 500000, got %d", stats.TopProducts[0].RevenuePaise)
	}

	// 4. List transactions with filter
	filtered, total, err := svc.ListTransactions(ctx, userID, &model.TransactionFilter{
		Platform: "shopify",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(filtered) != 3 {
		t.Errorf("expected 3 results, got %d", len(filtered))
	}

	// 5. List integrations (verify credentials hidden)
	integrations, err := svc.ListIntegrations(ctx, userID)
	if err != nil {
		t.Fatalf("list integrations: %v", err)
	}
	if len(integrations) != 1 {
		t.Fatalf("expected 1 integration, got %d", len(integrations))
	}
	if integrations[0].Credentials != nil {
		t.Error("credentials should be nil in list response")
	}

	// 6. Disconnect
	err = svc.DisconnectIntegration(ctx, userID, ci.ID)
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// Verify disconnected status
	integrations, _ = svc.ListIntegrations(ctx, userID)
	if integrations[0].Status != "disconnected" {
		t.Errorf("expected disconnected, got %s", integrations[0].Status)
	}
}

func TestListTransactions_Pagination(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Pagination Test", "page-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ci, err := svc.ConnectIntegration(ctx, userID, &model.ConnectCommerceRequest{
		Platform:        "gumroad",
		IntegrationType: "digital_product",
		Credentials:     map[string]string{"token": "t"},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	defer pool.Exec(ctx, `DELETE FROM commerce_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM commerce_integrations WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	now := time.Now()
	// Insert 5 transactions
	for i := 0; i < 5; i++ {
		extID := "gum-" + uuid.New().String()[:8]
		_ = svc.RecordTransaction(ctx, ci.ID, &model.CommerceTransaction{
			TransactionType:       "product_sale",
			ExternalTransactionID: &extID,
			AmountPaise:           int64((i + 1) * 100_00),
			Currency:              "INR",
			ProductName:           strPtr("Product"),
			TransactionDate:       now.Add(-time.Duration(i) * time.Hour),
		})
	}

	// Page 1, size 2
	page1, total, err := svc.ListTransactions(ctx, userID, &model.TransactionFilter{
		Page:     1,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(page1) != 2 {
		t.Errorf("expected 2 results on page 1, got %d", len(page1))
	}

	// Page 3, size 2 (should have 1 result)
	page3, total, err := svc.ListTransactions(ctx, userID, &model.TransactionFilter{
		Page:     3,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("list page 3: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(page3) != 1 {
		t.Errorf("expected 1 result on page 3, got %d", len(page3))
	}
}

func TestSyncIntegration_UpdatesLastSyncAt(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Sync Test", "sync-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ci, err := svc.ConnectIntegration(ctx, userID, &model.ConnectCommerceRequest{
		Platform:        "patreon",
		IntegrationType: "subscription",
		Credentials:     map[string]string{"token": "pat-token"},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	defer pool.Exec(ctx, `DELETE FROM commerce_integrations WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// Initial state: last_sync_at should be NULL
	integrations, _ := svc.ListIntegrations(ctx, userID)
	if integrations[0].LastSyncAt != nil {
		t.Error("expected initial last_sync_at to be nil")
	}

	// Sync (stub -- just updates timestamp)
	err = svc.SyncIntegration(ctx, ci.ID)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Verify last_sync_at is now set
	integrations, _ = svc.ListIntegrations(ctx, userID)
	if integrations[0].LastSyncAt == nil {
		t.Error("expected last_sync_at to be set after sync")
	}
}

func TestConnectIntegration_InvalidCombination(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Invalid Combo", "invalid-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// youtube + shopping is not valid
	_, err = svc.ConnectIntegration(ctx, userID, &model.ConnectCommerceRequest{
		Platform:        "youtube",
		IntegrationType: "shopping",
		Credentials:     map[string]string{"token": "t"},
	})
	if err == nil {
		t.Error("expected error for invalid platform/type combination")
	}
}

func TestDisconnectIntegration_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewCommerceService(pool)
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "Disconnect NF", "disc-"+userID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	err = svc.DisconnectIntegration(ctx, userID, uuid.New())
	if err == nil {
		t.Error("expected error for non-existent integration")
	}
}
