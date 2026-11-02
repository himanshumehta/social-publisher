package service

// ClampPagination enforces pagination limits.
// Default limit is 20, max limit is 100.
func ClampPagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
