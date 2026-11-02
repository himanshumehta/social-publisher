package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type NewsletterService struct {
	pool *pgxpool.Pool
}

func NewNewsletterService(pool *pgxpool.Pool) *NewsletterService {
	return &NewsletterService{pool: pool}
}

// ── Newsletter CRUD ──

func (s *NewsletterService) Create(ctx context.Context, userID uuid.UUID, req *model.CreateNewsletterRequest) (*model.Newsletter, error) {
	colorPrimary := "#2563EB"
	if req.ColorPrimary != "" {
		colorPrimary = req.ColorPrimary
	}
	colorSecondary := "#1E40AF"
	if req.ColorSecondary != "" {
		colorSecondary = req.ColorSecondary
	}

	nl := &model.Newsletter{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO newsletters (user_id, name, description, from_name, from_email,
			reply_to, color_primary, color_secondary, footer_text)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, user_id, name, description, from_name, from_email, reply_to,
			logo_url, color_primary, color_secondary, footer_text, is_active,
			subscriber_count, created_at, updated_at`,
		userID, req.Name, strPtrOrNil(req.Description), strPtrOrNil(req.FromName),
		strPtrOrNil(req.FromEmail), strPtrOrNil(req.ReplyTo),
		colorPrimary, colorSecondary, strPtrOrNil(req.FooterText),
	).Scan(
		&nl.ID, &nl.UserID, &nl.Name, &nl.Description, &nl.FromName,
		&nl.FromEmail, &nl.ReplyTo, &nl.LogoURL, &nl.ColorPrimary,
		&nl.ColorSecondary, &nl.FooterText, &nl.IsActive,
		&nl.SubscriberCount, &nl.CreatedAt, &nl.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create newsletter: %w", err)
	}
	return nl, nil
}

func (s *NewsletterService) Get(ctx context.Context, newsletterID uuid.UUID) (*model.Newsletter, error) {
	nl := &model.Newsletter{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, description, from_name, from_email, reply_to,
			logo_url, color_primary, color_secondary, footer_text, is_active,
			subscriber_count, created_at, updated_at
		FROM newsletters WHERE id = $1`, newsletterID,
	).Scan(
		&nl.ID, &nl.UserID, &nl.Name, &nl.Description, &nl.FromName,
		&nl.FromEmail, &nl.ReplyTo, &nl.LogoURL, &nl.ColorPrimary,
		&nl.ColorSecondary, &nl.FooterText, &nl.IsActive,
		&nl.SubscriberCount, &nl.CreatedAt, &nl.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get newsletter: %w", err)
	}
	return nl, nil
}

func (s *NewsletterService) ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.Newsletter, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, description, from_name, from_email, reply_to,
			logo_url, color_primary, color_secondary, footer_text, is_active,
			subscriber_count, created_at, updated_at
		FROM newsletters WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list newsletters: %w", err)
	}
	defer rows.Close()

	var newsletters []*model.Newsletter
	for rows.Next() {
		nl := &model.Newsletter{}
		if err := rows.Scan(
			&nl.ID, &nl.UserID, &nl.Name, &nl.Description, &nl.FromName,
			&nl.FromEmail, &nl.ReplyTo, &nl.LogoURL, &nl.ColorPrimary,
			&nl.ColorSecondary, &nl.FooterText, &nl.IsActive,
			&nl.SubscriberCount, &nl.CreatedAt, &nl.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan newsletter: %w", err)
		}
		newsletters = append(newsletters, nl)
	}
	return newsletters, nil
}

func (s *NewsletterService) Update(ctx context.Context, userID, newsletterID uuid.UUID, req *model.UpdateNewsletterRequest) (*model.Newsletter, error) {
	nl := &model.Newsletter{}
	err := s.pool.QueryRow(ctx, `
		UPDATE newsletters SET
			name = COALESCE($3, name),
			description = COALESCE($4, description),
			from_name = COALESCE($5, from_name),
			from_email = COALESCE($6, from_email),
			reply_to = COALESCE($7, reply_to),
			color_primary = COALESCE($8, color_primary),
			color_secondary = COALESCE($9, color_secondary),
			footer_text = COALESCE($10, footer_text),
			is_active = COALESCE($11, is_active),
			updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, name, description, from_name, from_email, reply_to,
			logo_url, color_primary, color_secondary, footer_text, is_active,
			subscriber_count, created_at, updated_at`,
		newsletterID, userID, req.Name, req.Description, req.FromName,
		req.FromEmail, req.ReplyTo, req.ColorPrimary, req.ColorSecondary,
		req.FooterText, req.IsActive,
	).Scan(
		&nl.ID, &nl.UserID, &nl.Name, &nl.Description, &nl.FromName,
		&nl.FromEmail, &nl.ReplyTo, &nl.LogoURL, &nl.ColorPrimary,
		&nl.ColorSecondary, &nl.FooterText, &nl.IsActive,
		&nl.SubscriberCount, &nl.CreatedAt, &nl.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update newsletter: %w", err)
	}
	return nl, nil
}

func (s *NewsletterService) Delete(ctx context.Context, userID, newsletterID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM newsletters WHERE id = $1 AND user_id = $2`, newsletterID, userID)
	if err != nil {
		return fmt.Errorf("delete newsletter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("newsletter not found")
	}
	return nil
}

// ── Subscribers ──

func (s *NewsletterService) AddSubscriber(ctx context.Context, newsletterID uuid.UUID, req *model.AddSubscriberRequest) (*model.NewsletterSubscriber, error) {
	source := "manual"
	if req.Source != "" {
		source = req.Source
	}

	sub := &model.NewsletterSubscriber{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO newsletter_subscribers (newsletter_id, email, name, source)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (newsletter_id, email) DO UPDATE SET
			status = 'active', unsubscribed_at = NULL
		RETURNING id, newsletter_id, email, name, status, source, metadata,
			subscribed_at, unsubscribed_at, created_at`,
		newsletterID, req.Email, strPtrOrNil(req.Name), source,
	).Scan(
		&sub.ID, &sub.NewsletterID, &sub.Email, &sub.Name, &sub.Status,
		&sub.Source, &sub.Metadata, &sub.SubscribedAt, &sub.UnsubscribedAt, &sub.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("add subscriber: %w", err)
	}

	// Update subscriber count
	_, _ = s.pool.Exec(ctx, `
		UPDATE newsletters SET subscriber_count = (
			SELECT COUNT(*) FROM newsletter_subscribers WHERE newsletter_id = $1 AND status = 'active'
		), updated_at = NOW() WHERE id = $1`, newsletterID)

	return sub, nil
}

func (s *NewsletterService) BulkAddSubscribers(ctx context.Context, newsletterID uuid.UUID, req *model.BulkAddSubscribersRequest) (int, error) {
	count := 0
	for _, sub := range req.Subscribers {
		_, err := s.AddSubscriber(ctx, newsletterID, &sub)
		if err == nil {
			count++
		}
	}

	return count, nil
}

func (s *NewsletterService) ListSubscribers(ctx context.Context, newsletterID uuid.UUID, status string) ([]*model.NewsletterSubscriber, error) {
	query := `
		SELECT id, newsletter_id, email, name, status, source, metadata,
			subscribed_at, unsubscribed_at, created_at
		FROM newsletter_subscribers WHERE newsletter_id = $1`
	args := []any{newsletterID}

	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY subscribed_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list subscribers: %w", err)
	}
	defer rows.Close()

	var subs []*model.NewsletterSubscriber
	for rows.Next() {
		sub := &model.NewsletterSubscriber{}
		if err := rows.Scan(
			&sub.ID, &sub.NewsletterID, &sub.Email, &sub.Name, &sub.Status,
			&sub.Source, &sub.Metadata, &sub.SubscribedAt, &sub.UnsubscribedAt, &sub.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan subscriber: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, nil
}

func (s *NewsletterService) Unsubscribe(ctx context.Context, newsletterID uuid.UUID, email string) error {
	now := time.Now()
	tag, err := s.pool.Exec(ctx, `
		UPDATE newsletter_subscribers SET status = 'unsubscribed', unsubscribed_at = $3
		WHERE newsletter_id = $1 AND email = $2 AND status = 'active'`,
		newsletterID, email, now)
	if err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("subscriber not found or already unsubscribed")
	}

	// Update subscriber count
	_, _ = s.pool.Exec(ctx, `
		UPDATE newsletters SET subscriber_count = (
			SELECT COUNT(*) FROM newsletter_subscribers WHERE newsletter_id = $1 AND status = 'active'
		), updated_at = NOW() WHERE id = $1`, newsletterID)

	return nil
}

func (s *NewsletterService) RemoveSubscriber(ctx context.Context, newsletterID, subscriberID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM newsletter_subscribers WHERE id = $1 AND newsletter_id = $2`,
		subscriberID, newsletterID)
	if err != nil {
		return fmt.Errorf("remove subscriber: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("subscriber not found")
	}

	// Update count
	_, _ = s.pool.Exec(ctx, `
		UPDATE newsletters SET subscriber_count = (
			SELECT COUNT(*) FROM newsletter_subscribers WHERE newsletter_id = $1 AND status = 'active'
		), updated_at = NOW() WHERE id = $1`, newsletterID)

	return nil
}

// ── Issues ──

func (s *NewsletterService) CreateIssue(ctx context.Context, newsletterID uuid.UUID, req *model.CreateIssueRequest) (*model.NewsletterIssue, error) {
	issue := &model.NewsletterIssue{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO newsletter_issues (newsletter_id, subject, preview_text,
			html_content, plain_text_content, source_post_id, source_project_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, newsletter_id, subject, preview_text, html_content,
			plain_text_content, status, source_post_id, source_project_id,
			sent_count, open_count, click_count, bounce_count,
			scheduled_at, sent_at, created_at, updated_at`,
		newsletterID, req.Subject, strPtrOrNil(req.PreviewText),
		req.HTMLContent, strPtrOrNil(req.PlainTextContent),
		req.SourcePostID, req.SourceProjectID,
	).Scan(
		&issue.ID, &issue.NewsletterID, &issue.Subject, &issue.PreviewText,
		&issue.HTMLContent, &issue.PlainTextContent, &issue.Status,
		&issue.SourcePostID, &issue.SourceProjectID,
		&issue.SentCount, &issue.OpenCount, &issue.ClickCount, &issue.BounceCount,
		&issue.ScheduledAt, &issue.SentAt, &issue.CreatedAt, &issue.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}
	return issue, nil
}

func (s *NewsletterService) GetIssue(ctx context.Context, issueID uuid.UUID) (*model.NewsletterIssue, error) {
	issue := &model.NewsletterIssue{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, newsletter_id, subject, preview_text, html_content,
			plain_text_content, status, source_post_id, source_project_id,
			sent_count, open_count, click_count, bounce_count,
			scheduled_at, sent_at, created_at, updated_at
		FROM newsletter_issues WHERE id = $1`, issueID,
	).Scan(
		&issue.ID, &issue.NewsletterID, &issue.Subject, &issue.PreviewText,
		&issue.HTMLContent, &issue.PlainTextContent, &issue.Status,
		&issue.SourcePostID, &issue.SourceProjectID,
		&issue.SentCount, &issue.OpenCount, &issue.ClickCount, &issue.BounceCount,
		&issue.ScheduledAt, &issue.SentAt, &issue.CreatedAt, &issue.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get issue: %w", err)
	}
	return issue, nil
}

func (s *NewsletterService) ListIssues(ctx context.Context, newsletterID uuid.UUID) ([]*model.NewsletterIssue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, newsletter_id, subject, preview_text, html_content,
			plain_text_content, status, source_post_id, source_project_id,
			sent_count, open_count, click_count, bounce_count,
			scheduled_at, sent_at, created_at, updated_at
		FROM newsletter_issues WHERE newsletter_id = $1 ORDER BY created_at DESC`, newsletterID)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	var issues []*model.NewsletterIssue
	for rows.Next() {
		issue := &model.NewsletterIssue{}
		if err := rows.Scan(
			&issue.ID, &issue.NewsletterID, &issue.Subject, &issue.PreviewText,
			&issue.HTMLContent, &issue.PlainTextContent, &issue.Status,
			&issue.SourcePostID, &issue.SourceProjectID,
			&issue.SentCount, &issue.OpenCount, &issue.ClickCount, &issue.BounceCount,
			&issue.ScheduledAt, &issue.SentAt, &issue.CreatedAt, &issue.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

func (s *NewsletterService) SendIssue(ctx context.Context, userID, issueID uuid.UUID) error {
	// Mark as sending
	tag, err := s.pool.Exec(ctx, `
		UPDATE newsletter_issues SET status = 'sending', updated_at = NOW()
		WHERE id = $1 AND status = 'draft'`, issueID)
	if err != nil {
		return fmt.Errorf("send issue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("issue not found or not in draft status")
	}

	// Count active subscribers for this issue's newsletter
	var sentCount int
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM newsletter_subscribers ns
		JOIN newsletter_issues ni ON ni.newsletter_id = ns.newsletter_id
		WHERE ni.id = $1 AND ns.status = 'active'`, issueID,
	).Scan(&sentCount)
	if err != nil {
		return fmt.Errorf("count subscribers: %w", err)
	}

	// Mark as sent
	now := time.Now()
	_, err = s.pool.Exec(ctx, `
		UPDATE newsletter_issues SET status = 'sent', sent_count = $2, sent_at = $3, updated_at = NOW()
		WHERE id = $1`, issueID, sentCount, now)
	if err != nil {
		return fmt.Errorf("complete send: %w", err)
	}

	return nil
}

func (s *NewsletterService) DeleteIssue(ctx context.Context, newsletterID, issueID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM newsletter_issues WHERE id = $1 AND newsletter_id = $2 AND status = 'draft'`,
		issueID, newsletterID)
	if err != nil {
		return fmt.Errorf("delete issue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("issue not found or not in draft status")
	}
	return nil
}

// ── Stats ──

func (s *NewsletterService) GetStats(ctx context.Context, newsletterID uuid.UUID) (*model.NewsletterStats, error) {
	stats := &model.NewsletterStats{}

	// Total and active subscribers
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'active')
		FROM newsletter_subscribers WHERE newsletter_id = $1`, newsletterID,
	).Scan(&stats.TotalSubscribers, &stats.ActiveSubscribers)
	if err != nil {
		return nil, fmt.Errorf("get subscriber stats: %w", err)
	}

	// Issue stats
	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'sent'),
			COALESCE(AVG(CASE WHEN sent_count > 0 THEN open_count::float / sent_count ELSE 0 END) FILTER (WHERE status = 'sent'), 0),
			COALESCE(AVG(CASE WHEN sent_count > 0 THEN click_count::float / sent_count ELSE 0 END) FILTER (WHERE status = 'sent'), 0)
		FROM newsletter_issues WHERE newsletter_id = $1`, newsletterID,
	).Scan(&stats.TotalIssuesSent, &stats.AvgOpenRate, &stats.AvgClickRate)
	if err != nil {
		return nil, fmt.Errorf("get issue stats: %w", err)
	}

	return stats, nil
}

// ── Scheduled Issue Picker (for worker) ──

func (s *NewsletterService) PickScheduledIssues(ctx context.Context) ([]*model.NewsletterIssue, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE newsletter_issues SET status = 'sending', updated_at = NOW()
		WHERE status = 'scheduled' AND scheduled_at <= NOW()
		RETURNING id, newsletter_id, subject, preview_text, html_content,
			plain_text_content, status, source_post_id, source_project_id,
			sent_count, open_count, click_count, bounce_count,
			scheduled_at, sent_at, created_at, updated_at`)
	if err != nil {
		return nil, fmt.Errorf("pick scheduled issues: %w", err)
	}
	defer rows.Close()

	var issues []*model.NewsletterIssue
	for rows.Next() {
		issue := &model.NewsletterIssue{}
		if err := rows.Scan(
			&issue.ID, &issue.NewsletterID, &issue.Subject, &issue.PreviewText,
			&issue.HTMLContent, &issue.PlainTextContent, &issue.Status,
			&issue.SourcePostID, &issue.SourceProjectID,
			&issue.SentCount, &issue.OpenCount, &issue.ClickCount, &issue.BounceCount,
			&issue.ScheduledAt, &issue.SentAt, &issue.CreatedAt, &issue.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan scheduled issue: %w", err)
		}
		issues = append(issues, issue)
	}
	return issues, nil
}
