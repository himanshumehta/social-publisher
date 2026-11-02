package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCreditsGetBalance_CreatesIfNotExists(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	// Create a test user for FK constraint.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "credits-test-user", "credits-test-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM ai_credit_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM ai_credits WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewCreditsService(pool)

	credits, err := svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if credits.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, credits.UserID)
	}
	if credits.Balance != 0 {
		t.Errorf("expected balance 0, got %d", credits.Balance)
	}
	if credits.TotalPurchased != 0 {
		t.Errorf("expected total_purchased 0, got %d", credits.TotalPurchased)
	}
	if credits.TotalUsed != 0 {
		t.Errorf("expected total_used 0, got %d", credits.TotalUsed)
	}
}

func TestCreditsAddCredits(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "credits-add-user", "credits-add-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM ai_credit_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM ai_credits WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewCreditsService(pool)

	// Add 100 credits.
	if err := svc.AddCredits(ctx, userID, 100, "test purchase"); err != nil {
		t.Fatalf("AddCredits failed: %v", err)
	}

	credits, err := svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if credits.Balance != 100 {
		t.Errorf("expected balance 100, got %d", credits.Balance)
	}
	if credits.TotalPurchased != 100 {
		t.Errorf("expected total_purchased 100, got %d", credits.TotalPurchased)
	}

	// Add another 50 credits.
	if err := svc.AddCredits(ctx, userID, 50, "bonus grant"); err != nil {
		t.Fatalf("AddCredits (second) failed: %v", err)
	}

	credits, err = svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance (after second add) failed: %v", err)
	}

	if credits.Balance != 150 {
		t.Errorf("expected balance 150, got %d", credits.Balance)
	}
	if credits.TotalPurchased != 150 {
		t.Errorf("expected total_purchased 150, got %d", credits.TotalPurchased)
	}
}

func TestCreditsDeduct_Success(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "credits-deduct-user", "credits-deduct-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM ai_credit_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM ai_credits WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewCreditsService(pool)

	// Seed 200 credits.
	if err := svc.AddCredits(ctx, userID, 200, "initial load"); err != nil {
		t.Fatalf("AddCredits failed: %v", err)
	}

	// Deduct 75 credits.
	if err := svc.Deduct(ctx, userID, 75, "ai generation", nil); err != nil {
		t.Fatalf("Deduct failed: %v", err)
	}

	credits, err := svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance after deduct failed: %v", err)
	}

	if credits.Balance != 125 {
		t.Errorf("expected balance 125, got %d", credits.Balance)
	}
	if credits.TotalUsed != 75 {
		t.Errorf("expected total_used 75, got %d", credits.TotalUsed)
	}
	if credits.TotalPurchased != 200 {
		t.Errorf("expected total_purchased 200, got %d", credits.TotalPurchased)
	}
}

func TestCreditsDeduct_InsufficientBalance(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "credits-insuf-user", "credits-insuf-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM ai_credit_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM ai_credits WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewCreditsService(pool)

	// Seed only 10 credits.
	if err := svc.AddCredits(ctx, userID, 10, "small grant"); err != nil {
		t.Fatalf("AddCredits failed: %v", err)
	}

	// Attempt to deduct 50 -- should fail.
	err = svc.Deduct(ctx, userID, 50, "expensive job", nil)
	if err == nil {
		t.Fatal("expected error for insufficient balance, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient") {
		t.Errorf("expected error containing 'insufficient', got: %v", err)
	}

	// Verify balance is unchanged.
	credits, err := svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance after failed deduct: %v", err)
	}
	if credits.Balance != 10 {
		t.Errorf("expected balance unchanged at 10, got %d", credits.Balance)
	}
}

func TestCreditsListTransactions(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "credits-list-user", "credits-list-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM ai_credit_transactions WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM ai_credits WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewCreditsService(pool)

	// Add credits.
	if err := svc.AddCredits(ctx, userID, 500, "purchase"); err != nil {
		t.Fatalf("AddCredits failed: %v", err)
	}

	// Deduct credits.
	if err := svc.Deduct(ctx, userID, 120, "video render", nil); err != nil {
		t.Fatalf("Deduct failed: %v", err)
	}

	txns, err := svc.ListTransactions(ctx, userID, 10)
	if err != nil {
		t.Fatalf("ListTransactions failed: %v", err)
	}

	if len(txns) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txns))
	}

	// Transactions are ordered DESC by created_at, so the deduction is first.
	debit := txns[0]
	credit := txns[1]

	if debit.Amount != -120 {
		t.Errorf("expected debit amount -120, got %d", debit.Amount)
	}
	if debit.BalanceAfter != 380 {
		t.Errorf("expected balance_after 380 on debit, got %d", debit.BalanceAfter)
	}
	if debit.Reason != "video render" {
		t.Errorf("expected debit reason 'video render', got %q", debit.Reason)
	}
	if debit.AIJobID != nil {
		t.Errorf("expected debit ai_job_id nil, got %v", debit.AIJobID)
	}

	if credit.Amount != 500 {
		t.Errorf("expected credit amount 500, got %d", credit.Amount)
	}
	if credit.BalanceAfter != 500 {
		t.Errorf("expected balance_after 500 on credit, got %d", credit.BalanceAfter)
	}
	if credit.Reason != "purchase" {
		t.Errorf("expected credit reason 'purchase', got %q", credit.Reason)
	}
	if credit.AIJobID != nil {
		t.Errorf("expected nil ai_job_id on credit, got %v", credit.AIJobID)
	}
}
