package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultInstanceAuditSettingsValues(t *testing.T) {
	got := DefaultInstanceAuditSettingsValues()

	assert.Equal(t, DefaultAuditRetentionWeeks, got.RetentionWeeks)
	assert.Equal(t, DefaultAuditCaptureClientInfo, got.CaptureIPUA)
	assert.GreaterOrEqual(t, got.RetentionWeeks, MinAuditRetentionWeeks)
	assert.LessOrEqual(t, got.RetentionWeeks, MaxAuditRetentionWeeks)
}

func TestInstanceAuditSettingsValues_RetentionWindow(t *testing.T) {
	assert.Equal(t, 14*24*time.Hour, InstanceAuditSettingsValues{RetentionWeeks: 2}.RetentionWindow())
	assert.Equal(t, 52*7*24*time.Hour, InstanceAuditSettingsValues{RetentionWeeks: 52}.RetentionWindow())
}
