package handler

import (
	"context"
	"log/slog"
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
		slog.Warn("readiness: database ping failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	result["database"] = "ok"

	// Check Redis (non-critical)
	if err := h.rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("readiness: redis ping failed", "error", err)
		result["redis"] = "degraded"
	} else {
		result["redis"] = "ok"
	}

	writeJSON(w, http.StatusOK, result)
}

// Health is the liveness probe: it answers as long as the process serves HTTP.
func Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
