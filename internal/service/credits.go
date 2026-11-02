package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type CreditsService struct {
	pool *pgxpool.Pool
}

func NewCreditsService(pool *pgxpool.Pool) *CreditsService {
	return &CreditsService{pool: pool}
}

type CreditTransaction struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Amount       int        `json:"amount"`
	BalanceAfter int        `json:"balance_after"`
	Reason       string     `json:"reason"`
	AIJobID      *uuid.UUID `json:"ai_job_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// GetBalance returns user's credit balance, creating a record if none exists.
func (s *CreditsService) GetBalance(ctx context.Context, userID uuid.UUID) (*model.AICredits, error) {
	var credits model.AICredits
	err := s.pool.QueryRow(ctx,
		`INSERT INTO ai_credits (user_id, balance, total_purchased, total_used)
		 VALUES ($1, 0, 0, 0)
		 ON CONFLICT (user_id) DO UPDATE SET updated_at = ai_credits.updated_at
		 RETURNING user_id, balance, total_purchased, total_used, updated_at`,
		userID,
	).Scan(&credits.UserID, &credits.Balance, &credits.TotalPurchased,
		&credits.TotalUsed, &credits.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get credits: %w", err)
	}
	return &credits, nil
}

// Deduct subtracts credits for an AI job. Returns error if insufficient balance.
func (s *CreditsService) Deduct(ctx context.Context, userID uuid.UUID, amount int, reason string, jobID *uuid.UUID) error {
	if amount <= 0 {
		return fmt.Errorf("deduct amount must be positive")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Ensure credit record exists and lock it
	var balance int
	err = tx.QueryRow(ctx,
		`INSERT INTO ai_credits (user_id, balance, total_purchased, total_used)
		 VALUES ($1, 0, 0, 0)
		 ON CONFLICT (user_id) DO UPDATE SET updated_at = NOW()
		 RETURNING balance`,
		userID,
	).Scan(&balance)
	if err != nil {
		return fmt.Errorf("get balance for deduct: %w", err)
	}

	if balance < amount {
		return fmt.Errorf("insufficient credits: have %d, need %d", balance, amount)
	}

	newBalance := balance - amount

	// Update balance
	_, err = tx.Exec(ctx,
		`UPDATE ai_credits
		 SET balance = $1, total_used = total_used + $2, updated_at = NOW()
		 WHERE user_id = $3`,
		newBalance, amount, userID)
	if err != nil {
		return fmt.Errorf("update credits: %w", err)
	}

	// Record transaction
	_, err = tx.Exec(ctx,
		`INSERT INTO ai_credit_transactions (user_id, amount, balance_after, reason, ai_job_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, -amount, newBalance, reason, jobID)
	if err != nil {
		return fmt.Errorf("record debit transaction: %w", err)
	}

	return tx.Commit(ctx)
}

// AddCredits adds credits to a user (purchase or admin grant).
func (s *CreditsService) AddCredits(ctx context.Context, userID uuid.UUID, amount int, reason string) error {
	if amount <= 0 {
		return fmt.Errorf("add amount must be positive")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Upsert credit record
	var newBalance int
	err = tx.QueryRow(ctx,
		`INSERT INTO ai_credits (user_id, balance, total_purchased, total_used)
		 VALUES ($1, $2, $2, 0)
		 ON CONFLICT (user_id) DO UPDATE
		     SET balance = ai_credits.balance + $2,
		         total_purchased = ai_credits.total_purchased + $2,
		         updated_at = NOW()
		 RETURNING balance`,
		userID, amount,
	).Scan(&newBalance)
	if err != nil {
		return fmt.Errorf("add credits: %w", err)
	}

	// Record transaction
	_, err = tx.Exec(ctx,
		`INSERT INTO ai_credit_transactions (user_id, amount, balance_after, reason)
		 VALUES ($1, $2, $3, $4)`,
		userID, amount, newBalance, reason)
	if err != nil {
		return fmt.Errorf("record credit transaction: %w", err)
	}

	return tx.Commit(ctx)
}

// ListTransactions returns recent credit transactions for a user.
func (s *CreditsService) ListTransactions(ctx context.Context, userID uuid.UUID, limit int) ([]CreditTransaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, amount, balance_after, reason, ai_job_id, created_at
		 FROM ai_credit_transactions
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query credit transactions: %w", err)
	}
	defer rows.Close()

	txns := []CreditTransaction{}
	for rows.Next() {
		var t CreditTransaction
		if err := rows.Scan(&t.ID, &t.UserID, &t.Amount, &t.BalanceAfter,
			&t.Reason, &t.AIJobID, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan credit transaction: %w", err)
		}
		txns = append(txns, t)
	}
	return txns, nil
}
