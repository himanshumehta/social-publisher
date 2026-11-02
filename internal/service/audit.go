package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type AuditService struct {
	pool *pgxpool.Pool
}

func NewAuditService(pool *pgxpool.Pool) *AuditService {
	return &AuditService{pool: pool}
}

type AuditEntry struct {
	UserID     *uuid.UUID
	Action     model.AuditAction
	EntityType string
	EntityID   *uuid.UUID
	IPAddress  string
	UserAgent  string
	RequestID  string
	OldValues  interface{}
	NewValues  interface{}
	Metadata   interface{}
}

func (s *AuditService) Log(ctx context.Context, entry AuditEntry) {
	oldJSON, _ := json.Marshal(entry.OldValues)
	newJSON, _ := json.Marshal(entry.NewValues)
	metaJSON, _ := json.Marshal(entry.Metadata)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_logs (user_id, action, entity_type, entity_id, ip_address, user_agent, request_id, old_values, new_values, metadata)
		 VALUES ($1, $2, $3, $4, $5::inet, $6, $7, $8, $9, $10)`,
		entry.UserID, entry.Action, entry.EntityType, entry.EntityID,
		nilIfEmptyAny(entry.IPAddress), entry.UserAgent, entry.RequestID,
		oldJSON, newJSON, metaJSON,
	)
	if err != nil {
		slog.Error("failed to write audit log", "error", err, "action", entry.Action)
	}
}

func nilIfEmptyAny(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
