package service

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors_Messages(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"ErrNotFound", ErrNotFound, "not found"},
		{"ErrForbidden", ErrForbidden, "forbidden"},
		{"ErrInvalidInput", ErrInvalidInput, "invalid input"},
		{"ErrConflict", ErrConflict, "conflict"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("%s.Error() = %q, want %q", tt.name, got, tt.wantMsg)
			}
		})
	}
}

func TestSentinelErrors_Is(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrNotFound", ErrNotFound},
		{"ErrForbidden", ErrForbidden},
		{"ErrInvalidInput", ErrInvalidInput},
		{"ErrConflict", ErrConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.err) {
				t.Errorf("errors.Is(%v, %v) = false, want true", tt.err, tt.err)
			}
		})
	}
}

func TestSentinelErrors_WrappedDetection(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
	}{
		{"wrapped ErrNotFound", ErrNotFound},
		{"wrapped ErrForbidden", ErrForbidden},
		{"wrapped ErrInvalidInput", ErrInvalidInput},
		{"wrapped ErrConflict", ErrConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := fmt.Errorf("operation failed: %w", tt.sentinel)
			if !errors.Is(wrapped, tt.sentinel) {
				t.Errorf("errors.Is(wrapped, %v) = false, want true", tt.sentinel)
			}
		})
	}
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	sentinels := []error{ErrNotFound, ErrForbidden, ErrInvalidInput, ErrConflict}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("errors.Is(%v, %v) = true, want false (distinct sentinels)", a, b)
			}
		}
	}
}
