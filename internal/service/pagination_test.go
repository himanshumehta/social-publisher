package service

import "testing"

func TestClampPagination(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "zero limit defaults to 20",
			limit:      0,
			offset:     0,
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "negative limit defaults to 20",
			limit:      -5,
			offset:     0,
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "limit over 100 clamps to 100",
			limit:      200,
			offset:     0,
			wantLimit:  100,
			wantOffset: 0,
		},
		{
			name:       "negative offset clamps to 0",
			limit:      10,
			offset:     -10,
			wantLimit:  10,
			wantOffset: 0,
		},
		{
			name:       "valid limit and offset pass through unchanged",
			limit:      50,
			offset:     25,
			wantLimit:  50,
			wantOffset: 25,
		},
		{
			name:       "boundary: limit=1 is valid minimum",
			limit:      1,
			offset:     0,
			wantLimit:  1,
			wantOffset: 0,
		},
		{
			name:       "boundary: limit=100 is valid maximum",
			limit:      100,
			offset:     0,
			wantLimit:  100,
			wantOffset: 0,
		},
		{
			name:       "boundary: limit=101 clamps to 100",
			limit:      101,
			offset:     0,
			wantLimit:  100,
			wantOffset: 0,
		},
		{
			name:       "boundary: offset=0 passes through",
			limit:      20,
			offset:     0,
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "large valid offset passes through",
			limit:      20,
			offset:     9999,
			wantLimit:  20,
			wantOffset: 9999,
		},
		{
			name:       "both limit and offset need clamping",
			limit:      -1,
			offset:     -1,
			wantLimit:  20,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit, gotOffset := ClampPagination(tt.limit, tt.offset)
			if gotLimit != tt.wantLimit {
				t.Errorf("ClampPagination(%d, %d) limit = %d, want %d",
					tt.limit, tt.offset, gotLimit, tt.wantLimit)
			}
			if gotOffset != tt.wantOffset {
				t.Errorf("ClampPagination(%d, %d) offset = %d, want %d",
					tt.limit, tt.offset, gotOffset, tt.wantOffset)
			}
		})
	}
}
