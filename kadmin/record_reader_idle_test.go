package kadmin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIdleReachedEnd(t *testing.T) {
	tests := []struct {
		name    string
		live    bool
		hwm     int64
		pending int
		want    bool
	}{
		{name: "stops once a response reported the high water mark", hwm: 10, want: true},
		{name: "keeps reading with pending messages", hwm: 10, pending: 3, want: false},
		{name: "keeps reading before a fetch reported a high water mark", hwm: 0, want: false},
		{name: "never stops live consumption", live: true, hwm: 10, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, idleReachedEnd(tt.live, tt.hwm, tt.pending))
		})
	}
}

func TestRankForExtension(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	oldest := map[int]time.Time{
		1: now.Add(-time.Hour),
		2: now.Add(-time.Minute),
		3: now.Add(-time.Hour),
	}
	// partition 0 has no record yet, then the youngest oldest record, ties by partition
	assert.Equal(t, []int{0, 2, 1, 3}, rankForExtension([]int{0, 1, 2, 3}, oldest))
}

func TestNextWindow(t *testing.T) {
	t.Run("backward", func(t *testing.T) {
		ext, covered, ok := nextWindow(readingOffsets{start: 100, end: 119}, offsets{start: 0, end: 200}, true, 5)
		assert.True(t, ok)
		assert.Equal(t, readingOffsets{start: 95, end: 99}, ext)
		assert.Equal(t, readingOffsets{start: 95, end: 119}, covered)
	})
	t.Run("backward stops at the first offset", func(t *testing.T) {
		ext, covered, ok := nextWindow(readingOffsets{start: 2, end: 19}, offsets{start: 0, end: 200}, true, 5)
		assert.True(t, ok)
		assert.Equal(t, readingOffsets{start: 0, end: 1}, ext)
		assert.Equal(t, readingOffsets{start: 0, end: 19}, covered)
		_, _, ok = nextWindow(covered, offsets{start: 0, end: 200}, true, 5)
		assert.False(t, ok)
	})
	t.Run("forward", func(t *testing.T) {
		ext, covered, ok := nextWindow(readingOffsets{start: 0, end: 9}, offsets{start: 0, end: 20}, false, 4)
		assert.True(t, ok)
		assert.Equal(t, readingOffsets{start: 10, end: 13}, ext)
		assert.Equal(t, readingOffsets{start: 0, end: 13}, covered)
	})
}

func TestExtensionCandidates(t *testing.T) {
	all := map[int]offsets{0: {start: 0, end: 50}, 1: {start: 0, end: 50}, 2: {start: 0, end: 50}}
	windows := map[int]readingOffsets{
		0: {start: 0, end: 49},  // read from the beginning, nothing older
		1: {start: 30, end: 49}, // older offsets exist
		2: {start: 10, end: 49}, // older offsets exist
	}
	assert.Equal(t, []int{1, 2}, extensionCandidates([]int{0, 1, 2}, windows, all, true))
}
