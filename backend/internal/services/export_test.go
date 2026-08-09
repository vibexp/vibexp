package services

import "time"

// SetClockForTest overrides the reference clock used for recency decay so tests
// can assert deterministic ordering. Test-only.
func (s *SearchService) SetClockForTest(now func() time.Time) {
	s.now = now
}

// ExportedFreshnessFilter exposes freshnessFilter to the external test package.
// It converts the service-layer filter string into the repository's optional
// pointer, and it is the seam where an unrecognized value becomes "no filter".
func ExportedFreshnessFilter(value string) *string { return freshnessFilter(value) }
