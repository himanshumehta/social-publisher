package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type CommerceService struct {
	pool *pgxpool.Pool
}

func NewCommerceService(pool *pgxpool.Pool) *CommerceService {
	return &CommerceService{pool: pool}
}

// ── Valid platform/integration combinations ───────────────────────────

var validPlatformIntegrations = map[string]map[string]bool{
	"youtube":   {"super_chat": true, "membership": true},
	"instagram": {"shopping": true},
	"shopify":   {"shopping": true, "digital_product": true},
	"gumroad":   {"digital_product": true},
	"patreon":   {"subscription": true, "membership": true},
}

func validatePlatformIntegrationType(platform, intType string) bool {
	types, ok := validPlatformIntegrations[platform]
	if !ok {
		return false
	}
	return types[intType]
}

// ── ConnectIntegration ────────────────────────────────────────────────

func (s *CommerceService) ConnectIntegration(ctx context.Context, userID uuid.UUID, req *model.ConnectCommerceRequest) (*model.CommerceIntegration, error) {
	if req.Platform == "" || req.IntegrationType == "" {
		return nil, fmt.Errorf("platform and integration_type are required")
	}
	if !validatePlatformIntegrationType(req.Platform, req.IntegrationType) {
		return nil, fmt.Errorf("invalid platform/integration_type combination: %s/%s", req.Platform, req.IntegrationType)
	}

	// Serialize credentials to JSONB (TODO: encrypt before storing)
	credJSON, err := json.Marshal(req.Credentials)
	if err != nil {
		return nil, fmt.Errorf("marshal credentials: %w", err)
	}

	var ci model.CommerceIntegration
	err = s.pool.QueryRow(ctx,
		`INSERT INTO commerce_integrations
		    (user_id, channel_id, platform, integration_type, credentials, status)
		 VALUES ($1, $2, $3, $4, $5, 'active')
		 RETURNING id, user_id, channel_id, platform, integration_type, status,
		           last_sync_at, sync_error, created_at, updated_at`,
		userID, req.ChannelID, req.Platform, req.IntegrationType, credJSON,
	).Scan(
		&ci.ID, &ci.UserID, &ci.ChannelID, &ci.Platform, &ci.IntegrationType,
		&ci.Status, &ci.LastSyncAt, &ci.SyncError, &ci.CreatedAt, &ci.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("connect integration: %w", err)
	}
	// Never return credentials
	return &ci, nil
}

// ── DisconnectIntegration ─────────────────────────────────────────────

func (s *CommerceService) DisconnectIntegration(ctx context.Context, userID, integrationID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE commerce_integrations
		 SET status = 'disconnected', updated_at = NOW()
		 WHERE id = $1 AND user_id = $2`,
		integrationID, userID,
	)
	if err != nil {
		return fmt.Errorf("disconnect integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("integration not found")
	}
	return nil
}

// ── ListIntegrations ──────────────────────────────────────────────────

func (s *CommerceService) ListIntegrations(ctx context.Context, userID uuid.UUID) ([]model.CommerceIntegration, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, channel_id, platform, integration_type, status,
		        last_sync_at, sync_error, created_at, updated_at
		 FROM commerce_integrations
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	defer rows.Close()

	var list []model.CommerceIntegration
	for rows.Next() {
		var ci model.CommerceIntegration
		if err := rows.Scan(
			&ci.ID, &ci.UserID, &ci.ChannelID, &ci.Platform, &ci.IntegrationType,
			&ci.Status, &ci.LastSyncAt, &ci.SyncError, &ci.CreatedAt, &ci.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan integration: %w", err)
		}
		// Credentials intentionally not loaded
		list = append(list, ci)
	}
	return list, nil
}

// ── GetStats ──────────────────────────────────────────────────────────

func (s *CommerceService) GetStats(ctx context.Context, userID uuid.UUID, start, end time.Time) (*model.CommerceStats, error) {
	stats := &model.CommerceStats{
		ByPlatform:  make(map[string]model.PlatformCommerce),
		ByType:      make(map[string]int64),
		TopProducts: []model.ProductStat{},
		PeriodStart: start,
		PeriodEnd:   end,
	}

	// Aggregate by platform
	rows, err := s.pool.Query(ctx,
		`SELECT platform, SUM(amount_paise) AS revenue, COUNT(*) AS cnt
		 FROM commerce_transactions
		 WHERE user_id = $1 AND transaction_date >= $2 AND transaction_date < $3
		 GROUP BY platform`,
		userID, start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("stats by platform: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var platform string
		var revenue int64
		var cnt int
		if err := rows.Scan(&platform, &revenue, &cnt); err != nil {
			return nil, fmt.Errorf("scan platform stats: %w", err)
		}
		stats.TotalRevenuePaise += revenue
		stats.TransactionCount += cnt
		stats.ByPlatform[platform] = model.PlatformCommerce{
			Platform:         platform,
			RevenuePaise:     revenue,
			TransactionCount: cnt,
		}
	}

	// Enrich platform stats with integration status and last_sync_at
	integrations, err := s.ListIntegrations(ctx, userID)
	if err == nil {
		for _, ci := range integrations {
			if pc, ok := stats.ByPlatform[ci.Platform]; ok {
				pc.LastSyncAt = ci.LastSyncAt
				pc.Status = ci.Status
				stats.ByPlatform[ci.Platform] = pc
			}
		}
	}

	// Aggregate by transaction type
	typeRows, err := s.pool.Query(ctx,
		`SELECT transaction_type, SUM(amount_paise) AS revenue
		 FROM commerce_transactions
		 WHERE user_id = $1 AND transaction_date >= $2 AND transaction_date < $3
		 GROUP BY transaction_type`,
		userID, start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("stats by type: %w", err)
	}
	defer typeRows.Close()

	for typeRows.Next() {
		var txnType string
		var revenue int64
		if err := typeRows.Scan(&txnType, &revenue); err != nil {
			return nil, fmt.Errorf("scan type stats: %w", err)
		}
		stats.ByType[txnType] = revenue
	}

	// Top products
	prodRows, err := s.pool.Query(ctx,
		`SELECT product_name, SUM(amount_paise) AS revenue, COUNT(*) AS cnt
		 FROM commerce_transactions
		 WHERE user_id = $1 AND transaction_date >= $2 AND transaction_date < $3
		   AND product_name IS NOT NULL AND product_name != ''
		 GROUP BY product_name
		 ORDER BY revenue DESC
		 LIMIT 10`,
		userID, start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("top products: %w", err)
	}
	defer prodRows.Close()

	for prodRows.Next() {
		var ps model.ProductStat
		if err := prodRows.Scan(&ps.Name, &ps.RevenuePaise, &ps.Count); err != nil {
			return nil, fmt.Errorf("scan product stat: %w", err)
		}
		stats.TopProducts = append(stats.TopProducts, ps)
	}

	return stats, nil
}

// ── ListTransactions ──────────────────────────────────────────────────

func (s *CommerceService) ListTransactions(ctx context.Context, userID uuid.UUID, filter *model.TransactionFilter) ([]model.CommerceTransaction, int, error) {
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	offset := (filter.Page - 1) * filter.PageSize

	// Build WHERE clauses dynamically
	where := "WHERE user_id = $1"
	args := []any{userID}
	argIdx := 2

	if filter.Platform != "" {
		where += fmt.Sprintf(" AND platform = $%d", argIdx)
		args = append(args, filter.Platform)
		argIdx++
	}
	if filter.TransactionType != "" {
		where += fmt.Sprintf(" AND transaction_type = $%d", argIdx)
		args = append(args, filter.TransactionType)
		argIdx++
	}
	if !filter.StartDate.IsZero() {
		where += fmt.Sprintf(" AND transaction_date >= $%d", argIdx)
		args = append(args, filter.StartDate)
		argIdx++
	}
	if !filter.EndDate.IsZero() {
		where += fmt.Sprintf(" AND transaction_date < $%d", argIdx)
		args = append(args, filter.EndDate)
		argIdx++
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM commerce_transactions " + where
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count transactions: %w", err)
	}

	// Fetch page
	dataQuery := fmt.Sprintf(
		`SELECT id, integration_id, user_id, platform, transaction_type,
		        external_transaction_id, amount_paise, currency,
		        customer_name, customer_id, product_name, metadata,
		        transaction_date, created_at
		 FROM commerce_transactions
		 %s
		 ORDER BY transaction_date DESC
		 LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, filter.PageSize, offset)

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	var list []model.CommerceTransaction
	for rows.Next() {
		var txn model.CommerceTransaction
		var metaBytes []byte
		if err := rows.Scan(
			&txn.ID, &txn.IntegrationID, &txn.UserID, &txn.Platform,
			&txn.TransactionType, &txn.ExternalTransactionID, &txn.AmountPaise,
			&txn.Currency, &txn.CustomerName, &txn.CustomerID, &txn.ProductName,
			&metaBytes, &txn.TransactionDate, &txn.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan transaction: %w", err)
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &txn.Metadata)
		}
		list = append(list, txn)
	}

	return list, total, nil
}

// ── RecordTransaction (upsert) ────────────────────────────────────────

func (s *CommerceService) RecordTransaction(ctx context.Context, integrationID uuid.UUID, txn *model.CommerceTransaction) error {
	metaJSON, err := json.Marshal(txn.Metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}

	// Look up user_id and platform from integration
	var userID uuid.UUID
	var platform string
	err = s.pool.QueryRow(ctx,
		`SELECT user_id, platform FROM commerce_integrations WHERE id = $1`,
		integrationID,
	).Scan(&userID, &platform)
	if err != nil {
		return fmt.Errorf("lookup integration: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO commerce_transactions
		    (integration_id, user_id, platform, transaction_type,
		     external_transaction_id, amount_paise, currency,
		     customer_name, customer_id, product_name, metadata, transaction_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (integration_id, external_transaction_id)
		 DO UPDATE SET
		    amount_paise = EXCLUDED.amount_paise,
		    customer_name = EXCLUDED.customer_name,
		    metadata = EXCLUDED.metadata`,
		integrationID, userID, platform, txn.TransactionType,
		txn.ExternalTransactionID, txn.AmountPaise, txn.Currency,
		txn.CustomerName, txn.CustomerID, txn.ProductName,
		metaJSON, txn.TransactionDate,
	)
	if err != nil {
		return fmt.Errorf("record transaction: %w", err)
	}
	return nil
}

// ── SyncIntegration ───────────────────────────────────────────────────

func (s *CommerceService) SyncIntegration(ctx context.Context, integrationID uuid.UUID) error {
	// Load the integration
	var ci model.CommerceIntegration
	var credBytes []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, channel_id, platform, integration_type, credentials,
		        status, last_sync_at, sync_error, created_at, updated_at
		 FROM commerce_integrations
		 WHERE id = $1`,
		integrationID,
	).Scan(
		&ci.ID, &ci.UserID, &ci.ChannelID, &ci.Platform, &ci.IntegrationType,
		&credBytes, &ci.Status, &ci.LastSyncAt, &ci.SyncError,
		&ci.CreatedAt, &ci.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("load integration for sync: %w", err)
	}

	// Deserialize credentials
	ci.Credentials = make(map[string]string)
	if len(credBytes) > 0 {
		_ = json.Unmarshal(credBytes, &ci.Credentials)
	}

	// Route to platform-specific sync stub
	var syncErr error
	switch ci.Platform + "/" + ci.IntegrationType {
	case "youtube/super_chat":
		syncErr = s.syncYouTubeSuperChats(ctx, &ci)
	case "youtube/membership":
		syncErr = s.syncYouTubeMemberships(ctx, &ci)
	case "instagram/shopping":
		syncErr = s.syncInstagramShopping(ctx, &ci)
	case "shopify/shopping", "shopify/digital_product":
		syncErr = s.syncShopifyOrders(ctx, &ci)
	case "gumroad/digital_product":
		syncErr = s.syncGumroadSales(ctx, &ci)
	case "patreon/subscription", "patreon/membership":
		syncErr = s.syncPatreonPledges(ctx, &ci)
	default:
		syncErr = fmt.Errorf("unsupported platform/type: %s/%s", ci.Platform, ci.IntegrationType)
	}

	// Update last_sync_at and sync_error
	var syncErrStr *string
	if syncErr != nil {
		s := syncErr.Error()
		syncErrStr = &s
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE commerce_integrations
		 SET last_sync_at = NOW(), sync_error = $2, updated_at = NOW()
		 WHERE id = $1`,
		integrationID, syncErrStr,
	)
	if err != nil {
		return fmt.Errorf("update sync status: %w", err)
	}

	return syncErr
}

// ── SyncAllActive ─────────────────────────────────────────────────────

func (s *CommerceService) SyncAllActive(ctx context.Context) error {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM commerce_integrations
		 WHERE status = 'active'
		   AND (last_sync_at IS NULL OR last_sync_at < NOW() - INTERVAL '1 hour')`,
	)
	if err != nil {
		return fmt.Errorf("find integrations to sync: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	var lastErr error
	for _, id := range ids {
		if err := s.SyncIntegration(ctx, id); err != nil {
			lastErr = err
			// Continue syncing other integrations even if one fails
		}
	}
	return lastErr
}

// ── Platform-specific sync stubs ──────────────────────────────────────
// Each returns nil for now. Real implementations will call external APIs
// and feed transactions via RecordTransaction.

func (s *CommerceService) syncYouTubeSuperChats(ctx context.Context, integration *model.CommerceIntegration) error {
	// TODO: Use YouTube Live Streaming API to fetch super chat events.
	// GET https://www.googleapis.com/youtube/v3/superChatEvents
	// Requires OAuth token from integration.Credentials["access_token"].
	_ = ctx
	_ = integration
	return nil
}

func (s *CommerceService) syncYouTubeMemberships(ctx context.Context, integration *model.CommerceIntegration) error {
	// TODO: Use YouTube Data API to fetch membership levels + members.
	// GET https://www.googleapis.com/youtube/v3/membershipsLevels
	// GET https://www.googleapis.com/youtube/v3/members
	_ = ctx
	_ = integration
	return nil
}

func (s *CommerceService) syncInstagramShopping(ctx context.Context, integration *model.CommerceIntegration) error {
	// TODO: Use Instagram Graph API / Commerce API to fetch product catalog orders.
	// Requires Facebook Business access token.
	_ = ctx
	_ = integration
	return nil
}

func (s *CommerceService) syncShopifyOrders(ctx context.Context, integration *model.CommerceIntegration) error {
	// TODO: Use Shopify Admin API (REST or GraphQL) to fetch orders.
	// GET https://{shop}.myshopify.com/admin/api/2024-01/orders.json
	// Requires integration.Credentials["api_key"] and ["api_secret"].
	_ = ctx
	_ = integration
	return nil
}

func (s *CommerceService) syncGumroadSales(ctx context.Context, integration *model.CommerceIntegration) error {
	// TODO: Use Gumroad API to fetch sales.
	// GET https://api.gumroad.com/v2/sales
	// Requires integration.Credentials["access_token"].
	_ = ctx
	_ = integration
	return nil
}

func (s *CommerceService) syncPatreonPledges(ctx context.Context, integration *model.CommerceIntegration) error {
	// TODO: Use Patreon API v2 to fetch pledges/members.
	// GET https://www.patreon.com/api/oauth2/v2/campaigns/{id}/members
	// Requires integration.Credentials["access_token"].
	_ = ctx
	_ = integration
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────

// calculateTopProducts aggregates transactions by product_name, sorts by
// revenue descending, and returns up to `limit` results.
func calculateTopProducts(transactions []model.CommerceTransaction, limit int) []model.ProductStat {
	agg := make(map[string]*model.ProductStat)

	for _, txn := range transactions {
		if txn.ProductName == nil || *txn.ProductName == "" {
			continue
		}
		ps, ok := agg[*txn.ProductName]
		if !ok {
			ps = &model.ProductStat{Name: *txn.ProductName}
			agg[*txn.ProductName] = ps
		}
		ps.RevenuePaise += txn.AmountPaise
		ps.Count++
	}

	result := make([]model.ProductStat, 0, len(agg))
	for _, ps := range agg {
		result = append(result, *ps)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].RevenuePaise > result[j].RevenuePaise
	})

	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// scanIntegration is unused but kept for potential future row scanning.
var _ = func(row pgx.Row) (*model.CommerceIntegration, error) {
	var ci model.CommerceIntegration
	err := row.Scan(
		&ci.ID, &ci.UserID, &ci.ChannelID, &ci.Platform, &ci.IntegrationType,
		&ci.Status, &ci.LastSyncAt, &ci.SyncError, &ci.CreatedAt, &ci.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &ci, nil
}
