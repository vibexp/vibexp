package repositories

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/models"
)

type orderedObserver struct {
	name string
	log  *[]string
}

func (o orderedObserver) OnInstanceSettingsAudited(_ context.Context, e models.InstanceSettingsAuditEntry) {
	*o.log = append(*o.log, o.name+":"+e.Setting)
}

func TestInstanceSettingsAuditObservers_ReportsToEachInOrder(t *testing.T) {
	var log []string
	observers := InstanceSettingsAuditObservers{orderedObserver{"capture", &log}, orderedObserver{"recorder", &log}}

	observers.OnInstanceSettingsAudited(context.Background(), models.InstanceSettingsAuditEntry{Setting: "audit_log"})

	assert.Equal(t, []string{"capture:audit_log", "recorder:audit_log"}, log)
}
