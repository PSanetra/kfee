package kadmin

import (
	"sort"
	"sync"
	"time"
)

// oldestTimes remembers the timestamp of the oldest record read per partition.
type oldestTimes struct {
	mu sync.Mutex
	m  map[int]time.Time
}

func newOldestTimes() *oldestTimes {
	return &oldestTimes{m: make(map[int]time.Time)}
}

func (o *oldestTimes) observe(partition int, ts time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if cur, ok := o.m[partition]; !ok || ts.Before(cur) {
		o.m[partition] = ts
	}
}

func (o *oldestTimes) snapshot() map[int]time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	result := make(map[int]time.Time, len(o.m))
	for p, ts := range o.m {
		result[p] = ts
	}
	return result
}

// rankForExtension orders the partitions that get more records: partitions without a record
// first, then the youngest oldest record, ties broken by partition number.
func rankForExtension(candidates []int, oldest map[int]time.Time) []int {
	ranked := append([]int(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		a, aok := oldest[ranked[i]]
		b, bok := oldest[ranked[j]]
		if aok != bok {
			return !aok
		}
		if aok && !a.Equal(b) {
			return a.After(b)
		}
		return ranked[i] < ranked[j]
	})
	return ranked
}

// extensionCandidates returns the partitions that have offsets beyond the windows read so far,
// in partition order.
func extensionCandidates(partitions []int, windows map[int]readingOffsets, all map[int]offsets, backward bool) []int {
	var candidates []int
	for _, p := range partitions {
		off, window := all[p], windows[p]
		if off.start < 0 {
			continue
		}
		if backward && window.start > off.start {
			candidates = append(candidates, p)
		}
		if !backward && window.end < off.newest() {
			candidates = append(candidates, p)
		}
	}
	return candidates
}

// nextWindow returns the next window of up to size offsets beyond the covered window, going back
// or forward, and the window covered afterwards. ok is false when no offset is left.
func nextWindow(covered readingOffsets, off offsets, backward bool, size int64) (ext, newCovered readingOffsets, ok bool) {
	if backward {
		size = min(size, covered.start-off.start)
		if size <= 0 {
			return readingOffsets{}, covered, false
		}
		ext = readingOffsets{start: covered.start - size, end: covered.start - 1}
		return ext, readingOffsets{start: ext.start, end: covered.end}, true
	}
	size = min(size, off.newest()-covered.end)
	if size <= 0 {
		return readingOffsets{}, covered, false
	}
	ext = readingOffsets{start: covered.end + 1, end: covered.end + size}
	return ext, readingOffsets{start: covered.start, end: ext.end}, true
}
