package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DashboardService struct {
	pool *pgxpool.Pool
}

func NewDashboardService(pool *pgxpool.Pool) *DashboardService {
	return &DashboardService{pool: pool}
}

// --- Response types ---

type RecentActivity struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreatorDashboardStats struct {
	TotalProjects int              `json:"total_projects"`
	Published     int              `json:"published"`
	NeedsAction   int              `json:"needs_action"`
	TotalVideos   int              `json:"total_videos"`
	TotalViews    int64            `json:"total_views"`
	TotalEarnings int64            `json:"total_earnings"`
	Recent        []RecentActivity `json:"recent"`
}

type AdminDashboardStats struct {
	TotalCreators  int   `json:"total_creators"`
	TotalEditors   int   `json:"total_editors"`
	ActiveProjects int   `json:"active_projects"`
	PendingReview  int   `json:"pending_review"`
	PendingBanking int   `json:"pending_banking"`
	TotalChannels  int   `json:"total_channels"`
	TotalViews     int64 `json:"total_views"`
	TotalRevenue   int64 `json:"total_revenue"`
}

type ProjectPerformance struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	Status        string    `json:"status"`
	VideoCount    int       `json:"video_count"`
	TotalViews    int64     `json:"total_views"`
	TotalEarnings int64     `json:"total_earnings"`
	PayoutStatus  string    `json:"payout_status"`
}

type VideoPerformance struct {
	ID               uuid.UUID `json:"id"`
	Format           string    `json:"format"`
	FileName         string    `json:"file_name"`
	Status           string    `json:"status"`
	Views            int64     `json:"views"`
	WatchTimeMinutes int64     `json:"watch_time_minutes"`
	Likes            int       `json:"likes"`
	Earnings         int64     `json:"earnings"`
	PlatformCount    int       `json:"platform_count"`
}

type PlatformStats struct {
	Platform         string `json:"platform"`
	ChannelName      string `json:"channel_name"`
	PlatformURL      string `json:"platform_url"`
	Views            int64  `json:"views"`
	WatchTimeMinutes int64  `json:"watch_time_minutes"`
	Likes            int    `json:"likes"`
	CommentsCount    int    `json:"comments_count"`
	Shares           int    `json:"shares"`
	EstimatedRevenue int64  `json:"estimated_revenue"`
	AdRevenue        int64  `json:"ad_revenue"`
}

// --- Methods ---

func (s *DashboardService) GetCreatorStats(ctx context.Context, userID uuid.UUID) (*CreatorDashboardStats, error) {
	stats := &CreatorDashboardStats{}

	err := s.pool.QueryRow(ctx, `
		WITH my_projects AS (
			SELECT id, status FROM projects WHERE creator_id = $1
		),
		my_videos AS (
			SELECT v.id FROM videos v
			JOIN my_projects a ON v.project_id = a.id
			WHERE v.is_deleted = FALSE AND v.format != 'raw'
		),
		analytics AS (
			SELECT COALESCE(SUM(ya.views), 0) AS total_views,
			       COALESCE(SUM(ya.estimated_revenue_paise), 0) AS total_earnings
			FROM youtube_analytics ya
			JOIN publish_records pr ON ya.publish_record_id = pr.id
			JOIN my_projects a ON pr.project_id = a.id
		)
		SELECT
			(SELECT COUNT(*) FROM my_projects),
			(SELECT COUNT(*) FROM my_projects WHERE status = 'published'),
			(SELECT COUNT(*) FROM my_projects WHERE status = 'in_review'),
			(SELECT COUNT(*) FROM my_videos),
			(SELECT total_views FROM analytics),
			(SELECT total_earnings FROM analytics)
	`, userID).Scan(
		&stats.TotalProjects,
		&stats.Published,
		&stats.NeedsAction,
		&stats.TotalVideos,
		&stats.TotalViews,
		&stats.TotalEarnings,
	)
	if err != nil {
		return nil, fmt.Errorf("get creator stats: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, title, status::text, updated_at
		FROM projects WHERE creator_id = $1
		ORDER BY updated_at DESC LIMIT 5
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get recent activity: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r RecentActivity
		if err := rows.Scan(&r.ID, &r.Title, &r.Status, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan recent activity: %w", err)
		}
		stats.Recent = append(stats.Recent, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recent activity: %w", err)
	}
	if stats.Recent == nil {
		stats.Recent = []RecentActivity{}
	}

	return stats, nil
}

func (s *DashboardService) GetAdminStats(ctx context.Context) (*AdminDashboardStats, error) {
	stats := &AdminDashboardStats{}

	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE role = 'creator' AND is_active = TRUE),
			(SELECT COUNT(*) FROM users WHERE role = 'editor' AND is_active = TRUE),
			(SELECT COUNT(*) FROM projects WHERE status NOT IN ('draft', 'published', 'cancelled')),
			(SELECT COUNT(*) FROM projects WHERE status = 'in_review'),
			(SELECT COUNT(*) FROM banking_info WHERE status = 'submitted'),
			(SELECT COUNT(*) FROM channels WHERE is_active = TRUE),
			COALESCE((SELECT SUM(views) FROM youtube_analytics), 0),
			COALESCE((SELECT SUM(estimated_revenue_paise) FROM youtube_analytics), 0)
	`).Scan(
		&stats.TotalCreators,
		&stats.TotalEditors,
		&stats.ActiveProjects,
		&stats.PendingReview,
		&stats.PendingBanking,
		&stats.TotalChannels,
		&stats.TotalViews,
		&stats.TotalRevenue,
	)
	if err != nil {
		return nil, fmt.Errorf("get admin stats: %w", err)
	}

	return stats, nil
}

func (s *DashboardService) ListProjectPerformance(ctx context.Context, creatorID *uuid.UUID, limit, offset int) ([]ProjectPerformance, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var args []interface{}
	where := ""
	argIdx := 1

	if creatorID != nil {
		where = fmt.Sprintf("WHERE a.creator_id = $%d", argIdx)
		args = append(args, *creatorID)
		argIdx++
	}

	var total int
	countQ := "SELECT COUNT(*) FROM projects a"
	if where != "" {
		countQ += " " + where
	}
	if err := s.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT
			a.id, a.title, a.status::text,
			COUNT(DISTINCT v.id) FILTER (WHERE v.format != 'raw' AND v.is_deleted = FALSE) AS video_count,
			COALESCE(SUM(ya.views), 0) AS total_views,
			COALESCE(SUM(ya.estimated_revenue_paise), 0) AS total_earnings,
			COALESCE((SELECT p.status::text FROM payouts p WHERE p.creator_id = a.creator_id ORDER BY p.created_at DESC LIMIT 1), '') AS payout_status
		FROM projects a
		LEFT JOIN videos v ON v.project_id = a.id
		LEFT JOIN publish_records pr ON pr.project_id = a.id AND pr.video_id = v.id
		LEFT JOIN youtube_analytics ya ON ya.publish_record_id = pr.id
		%s
		GROUP BY a.id, a.title, a.status, a.creator_id, a.updated_at
		ORDER BY a.updated_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list project performance: %w", err)
	}
	defer rows.Close()

	var results []ProjectPerformance
	for rows.Next() {
		var ap ProjectPerformance
		if err := rows.Scan(&ap.ID, &ap.Title, &ap.Status, &ap.VideoCount, &ap.TotalViews, &ap.TotalEarnings, &ap.PayoutStatus); err != nil {
			return nil, 0, fmt.Errorf("scan project performance: %w", err)
		}
		results = append(results, ap)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating project performance: %w", err)
	}
	if results == nil {
		results = []ProjectPerformance{}
	}

	return results, total, nil
}

func (s *DashboardService) GetVideoPerformance(ctx context.Context, projectID uuid.UUID) ([]VideoPerformance, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			v.id, v.format::text, v.file_name, v.status::text,
			COALESCE(SUM(ya.views), 0) AS views,
			COALESCE(SUM(ya.watch_time_minutes), 0) AS watch_time_minutes,
			COALESCE(SUM(ya.likes), 0) AS likes,
			COALESCE(SUM(ya.estimated_revenue_paise), 0) AS earnings,
			COUNT(DISTINCT pr.id) AS platform_count
		FROM videos v
		LEFT JOIN publish_records pr ON pr.video_id = v.id
		LEFT JOIN youtube_analytics ya ON ya.publish_record_id = pr.id
		WHERE v.project_id = $1 AND v.format != 'raw' AND v.is_deleted = FALSE
		GROUP BY v.id, v.format, v.file_name, v.status, v.created_at
		ORDER BY v.created_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("get video performance: %w", err)
	}
	defer rows.Close()

	var results []VideoPerformance
	for rows.Next() {
		var vp VideoPerformance
		if err := rows.Scan(&vp.ID, &vp.Format, &vp.FileName, &vp.Status, &vp.Views, &vp.WatchTimeMinutes, &vp.Likes, &vp.Earnings, &vp.PlatformCount); err != nil {
			return nil, fmt.Errorf("scan video performance: %w", err)
		}
		results = append(results, vp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating video performance: %w", err)
	}
	if results == nil {
		results = []VideoPerformance{}
	}

	return results, nil
}

// --- Analytics response types ---

type StatusDistribution struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type TopProject struct {
	ID       uuid.UUID `json:"id"`
	Title    string    `json:"title"`
	Views    int64     `json:"views"`
	Earnings int64     `json:"earnings"`
	Status   string    `json:"status"`
}

type AdminAnalytics struct {
	TotalRevenue    int64                `json:"total_revenue"`
	TotalViews      int64                `json:"total_views"`
	ActiveCreators  int                  `json:"active_creators"`
	PublishedVideos int                  `json:"published_videos"`
	StatusDist      []StatusDistribution `json:"status_distribution"`
	TopProjects     []TopProject         `json:"top_projects"`
}

type CreatorAnalyticsItem struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Views     int64     `json:"views"`
	WatchTime int64     `json:"watch_time"`
	Likes     int       `json:"likes"`
	Earnings  int64     `json:"earnings"`
	Published string    `json:"published_at"`
}

type CreatorAnalyticsResponse struct {
	TotalViews    int64                  `json:"total_views"`
	TotalEarnings int64                  `json:"total_earnings"`
	TotalVideos   int                    `json:"total_videos"`
	AvgViews      int64                  `json:"avg_views"`
	Items         []CreatorAnalyticsItem `json:"items"`
}

func periodToDays(period string) int {
	switch period {
	case "7d":
		return 7
	case "30d":
		return 30
	case "90d":
		return 90
	default:
		return 365 * 10 // effectively all time
	}
}

func (s *DashboardService) GetAdminAnalytics(ctx context.Context, period string) (*AdminAnalytics, error) {
	days := periodToDays(period)
	cutoff := time.Now().AddDate(0, 0, -days)
	a := &AdminAnalytics{}

	// Aggregate KPIs
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(ya.estimated_revenue_paise), 0),
			COALESCE(SUM(ya.views), 0),
			(SELECT COUNT(DISTINCT creator_id) FROM projects WHERE creator_id IS NOT NULL AND updated_at >= $1),
			(SELECT COUNT(*) FROM projects WHERE status = 'published')
		FROM youtube_analytics ya
		WHERE ya.period_start >= $1
	`, cutoff).Scan(&a.TotalRevenue, &a.TotalViews, &a.ActiveCreators, &a.PublishedVideos)
	if err != nil {
		return nil, fmt.Errorf("admin analytics KPIs: %w", err)
	}

	// Status distribution
	rows, err := s.pool.Query(ctx, `
		SELECT status::text, COUNT(*) FROM projects GROUP BY status ORDER BY COUNT(*) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("status distribution: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sd StatusDistribution
		if err := rows.Scan(&sd.Status, &sd.Count); err != nil {
			return nil, fmt.Errorf("scan status: %w", err)
		}
		a.StatusDist = append(a.StatusDist, sd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating status distribution: %w", err)
	}
	if a.StatusDist == nil {
		a.StatusDist = []StatusDistribution{}
	}

	// Top projects by views
	topRows, err := s.pool.Query(ctx, `
		SELECT a.id, a.title, COALESCE(SUM(ya.views), 0), COALESCE(SUM(ya.estimated_revenue_paise), 0), a.status::text
		FROM projects a
		JOIN publish_records pr ON pr.project_id = a.id
		JOIN youtube_analytics ya ON ya.publish_record_id = pr.id
		WHERE ya.period_start >= $1
		GROUP BY a.id, a.title, a.status
		ORDER BY SUM(ya.views) DESC
		LIMIT 10
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("top projects: %w", err)
	}
	defer topRows.Close()
	for topRows.Next() {
		var t TopProject
		if err := topRows.Scan(&t.ID, &t.Title, &t.Views, &t.Earnings, &t.Status); err != nil {
			return nil, fmt.Errorf("scan top project: %w", err)
		}
		a.TopProjects = append(a.TopProjects, t)
	}
	if err := topRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating top projects: %w", err)
	}
	if a.TopProjects == nil {
		a.TopProjects = []TopProject{}
	}

	return a, nil
}

func (s *DashboardService) GetCreatorAnalytics(ctx context.Context, creatorID uuid.UUID, period string) (*CreatorAnalyticsResponse, error) {
	days := periodToDays(period)
	cutoff := time.Now().AddDate(0, 0, -days)
	resp := &CreatorAnalyticsResponse{}

	// Aggregate KPIs
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(ya.views), 0),
			COALESCE(SUM(ya.estimated_revenue_paise), 0),
			COUNT(DISTINCT v.id)
		FROM projects a
		JOIN videos v ON v.project_id = a.id AND v.format != 'raw' AND v.is_deleted = FALSE
		LEFT JOIN publish_records pr ON pr.project_id = a.id AND pr.video_id = v.id
		LEFT JOIN youtube_analytics ya ON ya.publish_record_id = pr.id AND ya.period_start >= $2
		WHERE a.creator_id = $1
	`, creatorID, cutoff).Scan(&resp.TotalViews, &resp.TotalEarnings, &resp.TotalVideos)
	if err != nil {
		return nil, fmt.Errorf("creator analytics KPIs: %w", err)
	}

	if resp.TotalVideos > 0 {
		resp.AvgViews = resp.TotalViews / int64(resp.TotalVideos)
	}

	// Per-project breakdown
	rows, err := s.pool.Query(ctx, `
		SELECT
			a.id, a.title,
			COALESCE(SUM(ya.views), 0),
			COALESCE(SUM(ya.watch_time_minutes), 0),
			COALESCE(SUM(ya.likes), 0)::INT,
			COALESCE(SUM(ya.estimated_revenue_paise), 0),
			COALESCE(MIN(pr.published_at)::TEXT, '')
		FROM projects a
		LEFT JOIN publish_records pr ON pr.project_id = a.id
		LEFT JOIN youtube_analytics ya ON ya.publish_record_id = pr.id AND ya.period_start >= $2
		WHERE a.creator_id = $1 AND a.status IN ('published', 'approved')
		GROUP BY a.id, a.title
		ORDER BY COALESCE(SUM(ya.views), 0) DESC
		LIMIT 20
	`, creatorID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("creator analytics items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item CreatorAnalyticsItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Views, &item.WatchTime, &item.Likes, &item.Earnings, &item.Published); err != nil {
			return nil, fmt.Errorf("scan creator item: %w", err)
		}
		resp.Items = append(resp.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating creator analytics: %w", err)
	}
	if resp.Items == nil {
		resp.Items = []CreatorAnalyticsItem{}
	}

	return resp, nil
}

func (s *DashboardService) GetPlatformStats(ctx context.Context, videoID uuid.UUID) ([]PlatformStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			c.platform, c.channel_name,
			COALESCE(pr.platform_url, ''),
			COALESCE(SUM(ya.views), 0),
			COALESCE(SUM(ya.watch_time_minutes), 0),
			COALESCE(SUM(ya.likes), 0),
			COALESCE(SUM(ya.comments_count), 0),
			COALESCE(SUM(ya.shares), 0),
			COALESCE(SUM(ya.estimated_revenue_paise), 0),
			COALESCE(SUM(ya.ad_revenue_paise), 0)
		FROM publish_records pr
		JOIN channels c ON c.id = pr.channel_id
		LEFT JOIN youtube_analytics ya ON ya.publish_record_id = pr.id
		WHERE pr.video_id = $1
		GROUP BY c.platform, c.channel_name, pr.platform_url, pr.created_at
		ORDER BY pr.created_at DESC
	`, videoID)
	if err != nil {
		return nil, fmt.Errorf("get platform stats: %w", err)
	}
	defer rows.Close()

	var results []PlatformStats
	for rows.Next() {
		var ps PlatformStats
		if err := rows.Scan(&ps.Platform, &ps.ChannelName, &ps.PlatformURL, &ps.Views, &ps.WatchTimeMinutes, &ps.Likes, &ps.CommentsCount, &ps.Shares, &ps.EstimatedRevenue, &ps.AdRevenue); err != nil {
			return nil, fmt.Errorf("scan platform stats: %w", err)
		}
		results = append(results, ps)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating platform stats: %w", err)
	}
	if results == nil {
		results = []PlatformStats{}
	}

	return results, nil
}

// --- Flow 9: Enhanced Dashboard Types ---

type EarningsTrendItem struct {
	Month string `json:"month"` // "Jan 2026"
	Gross int64  `json:"gross"` // paise
	Net   int64  `json:"net"`   // after 30% platform fee + 10% TDS
	Views int64  `json:"views"`
}

type TopVideoItem struct {
	VideoID     uuid.UUID `json:"video_id"`
	Title       string    `json:"title"` // project title
	FileName    string    `json:"file_name"`
	Views       int64     `json:"views"`
	WatchTime   int64     `json:"watch_time"` // minutes
	Earnings    int64     `json:"earnings"`   // paise
	Platform    string    `json:"platform"`
	PublishedAt string    `json:"published_at"`
}

type PlatformBreakdownItem struct {
	Platform string  `json:"platform"`
	Revenue  int64   `json:"revenue"` // paise
	Pct      float64 `json:"pct"`     // 0-100
}

type ActivityFeedItem struct {
	Type      string    `json:"type"` // "publish", "milestone", "payout", "review"
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type CurrentMonthPayout struct {
	Videos      []CurrentMonthVideo `json:"videos"`
	Subtotal    int64               `json:"subtotal"`     // gross paise
	PlatformFee int64               `json:"platform_fee"` // 30%
	TDS         int64               `json:"tds"`          // 10% of net
	NetPayout   int64               `json:"net_payout"`
	Status      string              `json:"status"` // pending, approved, processing, completed
}

type CurrentMonthVideo struct {
	VideoID  uuid.UUID `json:"video_id"`
	Title    string    `json:"title"`
	Views    int64     `json:"views"`
	Earnings int64     `json:"earnings"` // paise
}

// --- Flow 9: Enhanced Dashboard Methods ---

func (s *DashboardService) GetEarningsTrend(ctx context.Context, creatorID uuid.UUID, months int) ([]EarningsTrendItem, error) {
	if months <= 0 {
		months = 6
	}
	if months > 12 {
		months = 12
	}

	cutoff := time.Now().AddDate(0, -months, 0)

	rows, err := s.pool.Query(ctx, `
		SELECT
			TO_CHAR(ya.period_start, 'Mon YYYY') AS month_label,
			COALESCE(SUM(ya.estimated_revenue_paise), 0) AS gross,
			COALESCE(SUM(ya.views), 0) AS views
		FROM youtube_analytics ya
		JOIN publish_records pr ON ya.publish_record_id = pr.id
		JOIN projects p ON pr.project_id = p.id
		WHERE p.creator_id = $1 AND ya.period_start >= $2
		GROUP BY DATE_TRUNC('month', ya.period_start), TO_CHAR(ya.period_start, 'Mon YYYY')
		ORDER BY DATE_TRUNC('month', ya.period_start) ASC
	`, creatorID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("get earnings trend: %w", err)
	}
	defer rows.Close()

	var results []EarningsTrendItem
	for rows.Next() {
		var item EarningsTrendItem
		if err := rows.Scan(&item.Month, &item.Gross, &item.Views); err != nil {
			return nil, fmt.Errorf("scan earnings trend: %w", err)
		}
		// Net = gross * 0.63 (after 30% platform fee, then 10% TDS on remainder)
		item.Net = int64(float64(item.Gross) * 0.63)
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating earnings trend: %w", err)
	}
	if results == nil {
		results = []EarningsTrendItem{}
	}

	return results, nil
}

func (s *DashboardService) GetTopVideos(ctx context.Context, creatorID uuid.UUID, period string, limit int) ([]TopVideoItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	days := periodToDays(period)
	cutoff := time.Now().AddDate(0, 0, -days)

	rows, err := s.pool.Query(ctx, `
		SELECT
			v.id,
			p.title,
			v.file_name,
			COALESCE(SUM(ya.views), 0) AS views,
			COALESCE(SUM(ya.watch_time_minutes), 0) AS watch_time,
			COALESCE(SUM(ya.estimated_revenue_paise), 0) AS earnings,
			COALESCE(c.platform, '') AS platform,
			COALESCE(pr.published_at::TEXT, '') AS published_at
		FROM videos v
		JOIN projects p ON v.project_id = p.id
		JOIN publish_records pr ON pr.project_id = p.id AND pr.video_id = v.id
		LEFT JOIN youtube_analytics ya ON ya.publish_record_id = pr.id AND ya.period_start >= $2
		LEFT JOIN channels c ON c.id = pr.channel_id
		WHERE p.creator_id = $1 AND v.is_deleted = FALSE AND v.format != 'raw'
		GROUP BY v.id, p.title, v.file_name, c.platform, pr.published_at
		ORDER BY COALESCE(SUM(ya.estimated_revenue_paise), 0) DESC
		LIMIT $3
	`, creatorID, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("get top videos: %w", err)
	}
	defer rows.Close()

	var results []TopVideoItem
	for rows.Next() {
		var item TopVideoItem
		if err := rows.Scan(&item.VideoID, &item.Title, &item.FileName, &item.Views, &item.WatchTime, &item.Earnings, &item.Platform, &item.PublishedAt); err != nil {
			return nil, fmt.Errorf("scan top video: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating top videos: %w", err)
	}
	if results == nil {
		results = []TopVideoItem{}
	}

	return results, nil
}

func (s *DashboardService) GetPlatformBreakdown(ctx context.Context, creatorID uuid.UUID, period string) ([]PlatformBreakdownItem, error) {
	days := periodToDays(period)
	cutoff := time.Now().AddDate(0, 0, -days)

	rows, err := s.pool.Query(ctx, `
		SELECT
			COALESCE(c.platform, 'unknown') AS platform,
			COALESCE(SUM(ya.estimated_revenue_paise), 0) AS revenue
		FROM youtube_analytics ya
		JOIN publish_records pr ON ya.publish_record_id = pr.id
		JOIN projects p ON pr.project_id = p.id
		LEFT JOIN channels c ON c.id = pr.channel_id
		WHERE p.creator_id = $1 AND ya.period_start >= $2
		GROUP BY c.platform
		ORDER BY revenue DESC
	`, creatorID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("get platform breakdown: %w", err)
	}
	defer rows.Close()

	var results []PlatformBreakdownItem
	var totalRevenue int64
	for rows.Next() {
		var item PlatformBreakdownItem
		if err := rows.Scan(&item.Platform, &item.Revenue); err != nil {
			return nil, fmt.Errorf("scan platform breakdown: %w", err)
		}
		totalRevenue += item.Revenue
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating platform breakdown: %w", err)
	}

	// Calculate percentages
	if totalRevenue > 0 {
		for i := range results {
			results[i].Pct = float64(results[i].Revenue) / float64(totalRevenue) * 100.0
		}
	}

	if results == nil {
		results = []PlatformBreakdownItem{}
	}

	return results, nil
}

func (s *DashboardService) GetActivityFeed(ctx context.Context, creatorID uuid.UUID, limit int) ([]ActivityFeedItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	rows, err := s.pool.Query(ctx, `
		(
			SELECT
				'review' AS type,
				CONCAT('Project transitioned from ', r.from_status, ' to ', r.to_status) AS message,
				r.created_at AS timestamp
			FROM reviews r
			JOIN projects p ON r.project_id = p.id
			WHERE p.creator_id = $1
			ORDER BY r.created_at DESC
			LIMIT $2
		)
		UNION ALL
		(
			SELECT
				'payout' AS type,
				CONCAT('Payout of ', po.net_amount, ' paise — ', po.status) AS message,
				po.created_at AS timestamp
			FROM payouts po
			WHERE po.creator_id = $1
			ORDER BY po.created_at DESC
			LIMIT $2
		)
		UNION ALL
		(
			SELECT
				n.type AS type,
				n.message AS message,
				n.created_at AS timestamp
			FROM notifications n
			WHERE n.user_id = $1
			ORDER BY n.created_at DESC
			LIMIT $2
		)
		ORDER BY timestamp DESC
		LIMIT $2
	`, creatorID, limit)
	if err != nil {
		return nil, fmt.Errorf("get activity feed: %w", err)
	}
	defer rows.Close()

	var results []ActivityFeedItem
	for rows.Next() {
		var item ActivityFeedItem
		if err := rows.Scan(&item.Type, &item.Message, &item.Timestamp); err != nil {
			return nil, fmt.Errorf("scan activity feed: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating activity feed: %w", err)
	}
	if results == nil {
		results = []ActivityFeedItem{}
	}

	return results, nil
}

func (s *DashboardService) GetCurrentMonthPayout(ctx context.Context, creatorID uuid.UUID) (*CurrentMonthPayout, error) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)

	result := &CurrentMonthPayout{
		Videos: []CurrentMonthVideo{},
		Status: "pending",
	}

	// Get per-video earnings for this month
	rows, err := s.pool.Query(ctx, `
		SELECT
			v.id,
			p.title,
			COALESCE(SUM(ya.views), 0) AS views,
			COALESCE(SUM(ya.estimated_revenue_paise), 0) AS earnings
		FROM videos v
		JOIN projects p ON v.project_id = p.id
		JOIN publish_records pr ON pr.project_id = p.id AND pr.video_id = v.id
		JOIN youtube_analytics ya ON ya.publish_record_id = pr.id
		WHERE p.creator_id = $1
			AND ya.period_start >= $2
			AND ya.period_start < $3
			AND v.is_deleted = FALSE
			AND v.format != 'raw'
		GROUP BY v.id, p.title
		ORDER BY earnings DESC
	`, creatorID, monthStart, monthEnd)
	if err != nil {
		return nil, fmt.Errorf("get current month videos: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var vid CurrentMonthVideo
		if err := rows.Scan(&vid.VideoID, &vid.Title, &vid.Views, &vid.Earnings); err != nil {
			return nil, fmt.Errorf("scan current month video: %w", err)
		}
		result.Videos = append(result.Videos, vid)
		result.Subtotal += vid.Earnings
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating current month videos: %w", err)
	}

	// Calculate deductions: 30% platform fee, then 10% TDS on remainder
	result.PlatformFee = int64(float64(result.Subtotal) * 0.30)
	afterFee := result.Subtotal - result.PlatformFee
	result.TDS = int64(float64(afterFee) * 0.10)
	result.NetPayout = afterFee - result.TDS

	// Check if a payout record exists for this period
	var payoutStatus *string
	err = s.pool.QueryRow(ctx, `
		SELECT status::TEXT FROM payouts
		WHERE creator_id = $1
			AND period_start >= $2
			AND period_end <= $3
		ORDER BY created_at DESC
		LIMIT 1
	`, creatorID, monthStart, monthEnd).Scan(&payoutStatus)
	if err == nil && payoutStatus != nil {
		result.Status = *payoutStatus
	}

	return result, nil
}
