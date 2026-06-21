package services

import "time"

// SetClockForTest overrides the reference clock used for recency decay so tests
// can assert deterministic ordering. Test-only.
func (s *SearchService) SetClockForTest(now func() time.Time) {
	s.now = now
}
