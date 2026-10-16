package services

import (
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// SetClockForTest overrides the reference clock used for recency decay so tests
// can assert deterministic ordering. Test-only.
func (s *SearchService) SetClockForTest(now func() time.Time) {
	s.now = now
}

// ExportedFreshnessFilter exposes freshnessFilter to the external test package.
// It converts the service-layer filter string into the repository's optional
// pointer, and it is the seam where an unrecognized value becomes "no filter".
func ExportedFreshnessFilter(value string) *string { return freshnessFilter(value) }

// SetClockForTest overrides the clock the capture-switch cache expires by.
// Test-only.
func (c *AuditClientInfoCapture) SetClockForTest(now func() time.Time) {
	c.now = now
}

// AuditCaptureCacheTTL exposes how long the capture switch is cached.
const AuditCaptureCacheTTL = auditCaptureCacheTTL

// SetClockForTest overrides the clock the purge cutoff is computed from.
// Test-only.
func (s *AuditPurgeService) SetClockForTest(now func() time.Time) {
	s.now = now
}

// SetExportCapForTest overrides the audit export row cap. Test-only.
func (s *AdminAuditService) SetExportCapForTest(rowCap int) {
	s.exportCap = rowCap
}

// ExportedAuditExportCell renders one named column of an audit export row.
// Test-only.
func ExportedAuditExportCell(e models.AuditEvent, header string) string {
	for _, c := range adminAuditExportColumns() {
		if c.header == header {
			return c.value(e)
		}
	}
	return ""
}
