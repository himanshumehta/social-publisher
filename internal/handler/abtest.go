package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// CreateABTest handles POST /api/v1/ab-tests
func (h *Handler) CreateABTest(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateABTestRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	test, err := h.services.ABTest.CreateTest(r.Context(), userID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	test.Variants = safeSlice(test.Variants)
	writeJSON(w, http.StatusCreated, test)
}

// GetABTest handles GET /api/v1/ab-tests/{id}
func (h *Handler) GetABTest(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	testID, ok := h.requirePathUUID(w, r, "id", "ab test id")
	if !ok {
		return
	}

	test, err := h.services.ABTest.GetTest(r.Context(), userID, testID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	test.Variants = safeSlice(test.Variants)
	writeJSON(w, http.StatusOK, test)
}

// ListABTests handles GET /api/v1/ab-tests
func (h *Handler) ListABTests(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	tests, err := h.services.ABTest.ListTests(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for i := range tests {
		tests[i].Variants = safeSlice(tests[i].Variants)
	}
	writeJSON(w, http.StatusOK, safeSlice(tests))
}

// StartABTest handles POST /api/v1/ab-tests/{id}/start
func (h *Handler) StartABTest(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	testID, ok := h.requirePathUUID(w, r, "id", "ab test id")
	if !ok {
		return
	}

	if err := h.services.ABTest.StartTest(r.Context(), userID, testID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "running"})
}

// EvaluateABTest handles GET /api/v1/ab-tests/{id}/evaluate
func (h *Handler) EvaluateABTest(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	testID, ok := h.requirePathUUID(w, r, "id", "ab test id")
	if !ok {
		return
	}

	// Verify ownership.
	if _, err := h.services.ABTest.GetTest(r.Context(), userID, testID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	result, err := h.services.ABTest.EvaluateTest(r.Context(), testID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// SelectABTestWinner handles POST /api/v1/ab-tests/{id}/winner
func (h *Handler) SelectABTestWinner(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	testID, ok := h.requirePathUUID(w, r, "id", "ab test id")
	if !ok {
		return
	}

	var req model.SelectWinnerRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.ABTest.SelectWinner(r.Context(), userID, testID, req.VariantID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

// CancelABTest handles POST /api/v1/ab-tests/{id}/cancel
func (h *Handler) CancelABTest(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	testID, ok := h.requirePathUUID(w, r, "id", "ab test id")
	if !ok {
		return
	}

	if err := h.services.ABTest.CancelTest(r.Context(), userID, testID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
