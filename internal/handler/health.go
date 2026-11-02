package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
}

func NewHealthHandler(pool *pgxpool.Pool, rdb *redis.Client) *HealthHandler {
	return &HealthHandler{pool: pool, rdb: rdb}
}

func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	result := map[string]string{"status": "ok"}

	// Check DB (critical)
	if err := h.pool.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("db: %v", err))
		return
	}
	result["database"] = "ok"

	// Check Redis (non-critical)
	if err := h.rdb.Ping(ctx).Err(); err != nil {
		result["redis"] = fmt.Sprintf("degraded: %v", err)
	} else {
		result["redis"] = "ok"
	}

	writeJSON(w, http.StatusOK, result)
}
