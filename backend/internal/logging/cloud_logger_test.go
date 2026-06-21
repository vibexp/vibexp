package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServiceHook_DefaultValues(t *testing.T) {
	hook := NewServiceHook("", "")

	assert.Equal(t, "vibexp-api", hook.serviceName)
	assert.Equal(t, "local", hook.serviceVersion)
}

func TestNewServiceHook_CloudRunEnvironment(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision-00001")

	assert.Equal(t, "test-service", hook.serviceName)
	assert.Equal(t, "test-revision-00001", hook.serviceVersion)
}

func TestServiceHook_Levels(t *testing.T) {
	hook := NewServiceHook("", "")
	levels := hook.Levels()

	assert.Equal(t, logrus.AllLevels, levels)
}

func TestServiceHook_Fire(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")
	entry := &logrus.Entry{
		Data: logrus.Fields{},
	}

	err := hook.Fire(entry)

	require.NoError(t, err)
	assert.Equal(t, "test-service", entry.Data["service"])
	assert.Equal(t, "test-revision", entry.Data["version"])
}

func TestServiceHook_Fire_DoesNotOverrideExisting(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")
	entry := &logrus.Entry{
		Data: logrus.Fields{
			"service": "custom-service",
			"version": "custom-version",
		},
	}

	err := hook.Fire(entry)

	require.NoError(t, err)
	assert.Equal(t, "custom-service", entry.Data["service"])
	assert.Equal(t, "custom-version", entry.Data["version"])
}

func TestCloudFormatter_SeverityMapping(t *testing.T) {
	testCases := []struct {
		level            logrus.Level
		expectedSeverity string
	}{
		{logrus.TraceLevel, "DEBUG"},
		{logrus.DebugLevel, "DEBUG"},
		{logrus.InfoLevel, "INFO"},
		{logrus.WarnLevel, "WARNING"},
		{logrus.ErrorLevel, "ERROR"},
		{logrus.FatalLevel, "CRITICAL"},
		{logrus.PanicLevel, "EMERGENCY"},
	}

	for _, tc := range testCases {
		t.Run(tc.level.String(), func(t *testing.T) {
			formatter := NewCloudFormatter()
			entry := &logrus.Entry{
				Level:   tc.level,
				Message: "test message",
				Data:    logrus.Fields{},
			}

			output, err := formatter.Format(entry)
			require.NoError(t, err)

			var result map[string]interface{}
			err = json.Unmarshal(output, &result)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedSeverity, result["severity"])
		})
	}
}

func TestNewCloudLogger(t *testing.T) {
	logger := NewCloudLogger(CloudLoggerConfig{})

	assert.NotNil(t, logger)
	assert.IsType(t, &CloudFormatter{}, logger.Formatter)
	assert.Equal(t, logrus.InfoLevel, logger.Level)
}

func TestNewCloudLogger_DebugLevel(t *testing.T) {
	logger := NewCloudLogger(CloudLoggerConfig{LogLevel: "debug"})

	assert.Equal(t, logrus.DebugLevel, logger.Level)
}

func TestNewCloudLogger_WarnLevel(t *testing.T) {
	logger := NewCloudLogger(CloudLoggerConfig{LogLevel: "warn"})

	assert.Equal(t, logrus.WarnLevel, logger.Level)
}

func TestNewCloudLogger_ErrorLevel(t *testing.T) {
	logger := NewCloudLogger(CloudLoggerConfig{LogLevel: "error"})

	assert.Equal(t, logrus.ErrorLevel, logger.Level)
}

func TestCloudLogger_OutputFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(CloudLoggerConfig{
		ServiceName:    "test-api",
		ServiceVersion: "test-00001",
		Output:         &buf,
	})

	logger.WithFields(logrus.Fields{
		"request_id": "test-request-123",
		"user_id":    "user-456",
	}).Info("Test log message")

	var result map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Verify Cloud Logging fields
	assert.Equal(t, "test-api", result["service"])
	assert.Equal(t, "test-00001", result["version"])
	assert.Equal(t, "INFO", result["severity"])
	assert.Equal(t, "Test log message", result["message"])
	assert.Equal(t, "test-request-123", result["request_id"])
	assert.Equal(t, "user-456", result["user_id"])
	assert.NotEmpty(t, result["timestamp"])
}

func TestCloudLogger_TraceCorrelation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(CloudLoggerConfig{Output: &buf})

	traceID := "projects/test-project/traces/abc123def456"
	logger.WithField("logging.googleapis.com/trace", traceID).Info("Correlated log")

	var result map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, traceID, result["logging.googleapis.com/trace"])
}

// TestServiceHook_Fire_InjectsErrorReportingOnError verifies that ERROR-level
// entries get the Cloud Error Reporting discriminator (`@type`) and
// `serviceContext` so GCP can auto-group them.
func TestServiceHook_Fire_InjectsErrorReportingOnError(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")
	entry := &logrus.Entry{
		Level: logrus.ErrorLevel,
		Data:  logrus.Fields{},
	}

	require.NoError(t, hook.Fire(entry))

	assert.Equal(t, errorReportingType, entry.Data["@type"])

	svcCtx, ok := entry.Data["serviceContext"].(map[string]string)
	require.True(t, ok, "serviceContext should be a map[string]string")
	assert.Equal(t, "test-service", svcCtx["service"])
	assert.Equal(t, "test-revision", svcCtx["version"])
}

// TestServiceHook_Fire_NoErrorReportingForInfoOrWarn is the counter-test:
// only ERROR-and-above entries should carry the discriminator.
func TestServiceHook_Fire_NoErrorReportingForInfoOrWarn(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")

	for _, level := range []logrus.Level{
		logrus.TraceLevel,
		logrus.DebugLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
	} {
		t.Run(level.String(), func(t *testing.T) {
			entry := &logrus.Entry{
				Level: level,
				Data:  logrus.Fields{},
			}
			require.NoError(t, hook.Fire(entry))

			_, hasType := entry.Data["@type"]
			_, hasCtx := entry.Data["serviceContext"]
			assert.False(t, hasType, "%s entries must not carry @type", level)
			assert.False(t, hasCtx, "%s entries must not carry serviceContext", level)
		})
	}
}

// TestServiceHook_Fire_ErrorReportingOnHigherSeverities ensures FATAL and
// PANIC entries also pick up the Error Reporting discriminator (logrus
// orders these stricter than Error).
func TestServiceHook_Fire_ErrorReportingOnHigherSeverities(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")

	for _, level := range []logrus.Level{logrus.FatalLevel, logrus.PanicLevel} {
		t.Run(level.String(), func(t *testing.T) {
			entry := &logrus.Entry{
				Level: level,
				Data:  logrus.Fields{},
			}
			require.NoError(t, hook.Fire(entry))

			assert.Equal(
				t,
				"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent",
				entry.Data["@type"],
			)
			_, hasCtx := entry.Data["serviceContext"]
			assert.True(t, hasCtx, "%s entries should carry serviceContext", level)
		})
	}
}

// TestServiceHook_Fire_WrappedErrorMessageExpansion verifies that wrapped
// errors flowing through logrus.WithError get formatted with %+v on
// ERROR-and-above paths so any frames they carry (pkg/errors etc.) survive
// into the message field.
func TestServiceHook_Fire_WrappedErrorMessageExpansion(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")

	root := errors.New("root cause")
	wrapped := fmt.Errorf("outer: %w", root)

	entry := &logrus.Entry{
		Level:   logrus.ErrorLevel,
		Message: "operation failed",
		Data: logrus.Fields{
			"error": wrapped,
		},
	}

	require.NoError(t, hook.Fire(entry))

	assert.Contains(t, entry.Message, "operation failed")
	assert.Contains(t, entry.Message, "outer")
	assert.Contains(t, entry.Message, "root cause")
}

// TestServiceHook_Fire_NoExpansionForWarn confirms wrapped errors at WARN
// level are left alone so we don't leak verbose %+v output into routine
// warnings.
func TestServiceHook_Fire_NoExpansionForWarn(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")

	entry := &logrus.Entry{
		Level:   logrus.WarnLevel,
		Message: "warning",
		Data: logrus.Fields{
			"error": errors.New("a thing"),
		},
	}

	require.NoError(t, hook.Fire(entry))

	// Message must not have been rewritten.
	assert.Equal(t, "warning", entry.Message)
}

// TestServiceHook_Fire_NoExpansionForNonErrorValue ensures we don't try to
// format string-shaped error fields with %+v (they'd just print
// `%!+v(string=...)`).
func TestServiceHook_Fire_NoExpansionForNonErrorValue(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")

	entry := &logrus.Entry{
		Level:   logrus.ErrorLevel,
		Message: "operation failed",
		Data: logrus.Fields{
			"error": "not actually an error type",
		},
	}

	require.NoError(t, hook.Fire(entry))

	assert.Equal(t, "operation failed", entry.Message)
}

// TestServiceHook_Fire_PreservesExistingErrorReportingFields makes sure the
// hook does not overwrite caller-supplied @type or serviceContext.
func TestServiceHook_Fire_PreservesExistingErrorReportingFields(t *testing.T) {
	hook := NewServiceHook("test-service", "test-revision")

	entry := &logrus.Entry{
		Level: logrus.ErrorLevel,
		Data: logrus.Fields{
			"@type":          "custom-type",
			"serviceContext": map[string]string{"service": "custom"},
		},
	}

	require.NoError(t, hook.Fire(entry))

	assert.Equal(t, "custom-type", entry.Data["@type"])
	svcCtx, ok := entry.Data["serviceContext"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "custom", svcCtx["service"])
}

// TestCloudLogger_OutputFormat_ErrorIncludesErrorReporting drives the full
// logger chain (formatter + hook) and asserts the serialized JSON carries
// the Error Reporting fields with the right values when severity is ERROR.
func TestCloudLogger_OutputFormat_ErrorIncludesErrorReporting(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(CloudLoggerConfig{
		ServiceName:    "test-api",
		ServiceVersion: "test-00001",
		Output:         &buf,
	})

	logger.WithError(fmt.Errorf("outer: %w", errors.New("root cause"))).
		Error("Test error message")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "ERROR", result["severity"])
	assert.Equal(t, errorReportingType, result["@type"])

	svcCtx, ok := result["serviceContext"].(map[string]interface{})
	require.True(t, ok, "serviceContext should serialize as a JSON object")
	assert.Equal(t, "test-api", svcCtx["service"])
	assert.Equal(t, "test-00001", svcCtx["version"])

	message, _ := result["message"].(string)
	assert.Contains(t, message, "Test error message")
	assert.Contains(t, message, "root cause")

	// Structured Data["error"] must survive into output — Cloud Logging
	// filters such as jsonPayload.error rely on it staying in the payload.
	// (We don't assert its serialised shape here because logrus' rendering of
	// `error` interface values via json.Marshal is implementation-defined.)
	assert.Contains(t, result, "error", "Data['error'] must survive serialization")
}

// TestCloudLogger_OutputFormat_InfoOmitsErrorReporting confirms INFO entries
// stay clean — Error Reporting must not pick them up.
func TestCloudLogger_OutputFormat_InfoOmitsErrorReporting(t *testing.T) {
	var buf bytes.Buffer
	logger := NewCloudLogger(CloudLoggerConfig{
		ServiceName:    "test-api",
		ServiceVersion: "test-00001",
		Output:         &buf,
	})

	logger.Info("routine event")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "INFO", result["severity"])
	_, hasType := result["@type"]
	_, hasCtx := result["serviceContext"]
	assert.False(t, hasType)
	assert.False(t, hasCtx)
}
