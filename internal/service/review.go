package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ReviewService struct {
	pool *pgxpool.Pool
}

func NewReviewService(pool *pgxpool.Pool) *ReviewService {
	return &ReviewService{pool: pool}
}

type CreateCommentRequest struct {
	ProjectID    uuid.UUID  `json:"project_id"`
	VideoID      *uuid.UUID `json:"video_id,omitempty"`
	ParentID     *uuid.UUID `json:"parent_id,omitempty"`
	Comment      string     `json:"comment"`
	TimestampSec *int       `json:"timestamp_sec,omitempty"`
}

func (s *ReviewService) CreateComment(ctx context.Context, reviewerID uuid.UUID, req CreateCommentRequest) (*model.Review, error) {
	if req.Comment == "" {
		return nil, fmt.Errorf("comment is required")
	}

	var r model.Review
	err := s.pool.QueryRow(ctx,
		`INSERT INTO reviews (project_id, video_id, reviewer_id, parent_id, action, comment, timestamp_sec)
		 VALUES ($1, $2, $3, $4, 'comment', $5, $6)
		 RETURNING id, project_id, video_id, reviewer_id, parent_id, action, comment, timestamp_sec, COALESCE(from_status::text, ''), COALESCE(to_status::text, ''), created_at`,
		req.ProjectID, req.VideoID, reviewerID, req.ParentID, req.Comment, req.TimestampSec,
	).Scan(&r.ID, &r.ProjectID, &r.VideoID, &r.ReviewerID, &r.ParentID,
		&r.Action, &r.Comment, &r.TimestampSec, &r.FromStatus, &r.ToStatus, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}

	// Get reviewer name
	_ = s.pool.QueryRow(ctx, `SELECT name FROM users WHERE id = $1`, reviewerID).Scan(&r.ReviewerName)

	return &r, nil
}

func (s *ReviewService) ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]model.Review, int, error) {
	limit, offset = ClampPagination(limit, offset)

	// Count top-level comments (no parent) for pagination
	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reviews WHERE project_id = $1 AND parent_id IS NULL`, projectID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count reviews: %w", err)
	}

	// Fetch paginated top-level comments
	topRows, err := s.pool.Query(ctx,
		`SELECT r.id, r.project_id, r.video_id, r.reviewer_id, r.parent_id,
		        r.action, r.comment, r.timestamp_sec, COALESCE(r.from_status::text, ''), COALESCE(r.to_status::text, ''),
		        r.created_at, u.name
		 FROM reviews r
		 JOIN users u ON u.id = r.reviewer_id
		 WHERE r.project_id = $1 AND r.parent_id IS NULL
		 ORDER BY r.created_at ASC
		 LIMIT $2 OFFSET $3`, projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query reviews: %w", err)
	}
	defer topRows.Close()

	var topLevel []model.Review
	var topIDs []uuid.UUID
	for topRows.Next() {
		var r model.Review
		if err := topRows.Scan(&r.ID, &r.ProjectID, &r.VideoID, &r.ReviewerID, &r.ParentID,
			&r.Action, &r.Comment, &r.TimestampSec, &r.FromStatus, &r.ToStatus,
			&r.CreatedAt, &r.ReviewerName); err != nil {
			return nil, 0, fmt.Errorf("scan review: %w", err)
		}
		r.Replies = []model.Review{}
		topLevel = append(topLevel, r)
		topIDs = append(topIDs, r.ID)
	}
	if err = topRows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate reviews: %w", err)
	}

	// Fetch all replies for the paginated top-level comments
	if len(topIDs) > 0 {
		replyRows, err := s.pool.Query(ctx,
			`SELECT r.id, r.project_id, r.video_id, r.reviewer_id, r.parent_id,
			        r.action, r.comment, r.timestamp_sec, COALESCE(r.from_status::text, ''), COALESCE(r.to_status::text, ''),
			        r.created_at, u.name
			 FROM reviews r
			 JOIN users u ON u.id = r.reviewer_id
			 WHERE r.project_id = $1 AND r.parent_id = ANY($2)
			 ORDER BY r.created_at ASC`, projectID, topIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("query reply reviews: %w", err)
		}
		defer replyRows.Close()

		// Map parent_id -> replies
		replyMap := make(map[uuid.UUID][]model.Review)
		for replyRows.Next() {
			var r model.Review
			if err := replyRows.Scan(&r.ID, &r.ProjectID, &r.VideoID, &r.ReviewerID, &r.ParentID,
				&r.Action, &r.Comment, &r.TimestampSec, &r.FromStatus, &r.ToStatus,
				&r.CreatedAt, &r.ReviewerName); err != nil {
				return nil, 0, fmt.Errorf("scan reply review: %w", err)
			}
			r.Replies = []model.Review{}
			if r.ParentID != nil {
				replyMap[*r.ParentID] = append(replyMap[*r.ParentID], r)
			}
		}
		if err = replyRows.Err(); err != nil {
			return nil, 0, fmt.Errorf("iterate reply reviews: %w", err)
		}

		// Attach replies to their parents
		for i := range topLevel {
			if replies, ok := replyMap[topLevel[i].ID]; ok {
				topLevel[i].Replies = replies
			}
		}
	}

	if topLevel == nil {
		topLevel = []model.Review{}
	}
	return topLevel, total, nil
}

func (s *ReviewService) ListByVideo(ctx context.Context, videoID uuid.UUID, limit, offset int) ([]model.Review, int, error) {
	limit, offset = ClampPagination(limit, offset)

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reviews WHERE video_id = $1`, videoID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count video reviews: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT r.id, r.project_id, r.video_id, r.reviewer_id, r.parent_id,
		        r.action, r.comment, r.timestamp_sec, COALESCE(r.from_status::text, ''), COALESCE(r.to_status::text, ''),
		        r.created_at, u.name
		 FROM reviews r
		 JOIN users u ON u.id = r.reviewer_id
		 WHERE r.video_id = $1
		 ORDER BY r.timestamp_sec ASC NULLS LAST, r.created_at ASC
		 LIMIT $2 OFFSET $3`, videoID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query video reviews: %w", err)
	}
	defer rows.Close()

	var reviews []model.Review
	for rows.Next() {
		var r model.Review
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.VideoID, &r.ReviewerID, &r.ParentID,
			&r.Action, &r.Comment, &r.TimestampSec, &r.FromStatus, &r.ToStatus,
			&r.CreatedAt, &r.ReviewerName); err != nil {
			return nil, 0, fmt.Errorf("scan review: %w", err)
		}
		reviews = append(reviews, r)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate video reviews: %w", err)
	}
	if reviews == nil {
		reviews = []model.Review{}
	}
	return reviews, total, nil
}
