// Package logging provides Cloud Run optimized logging for Google Cloud Logging
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// CloudLoggerConfig holds configuration for the Cloud Logger
type CloudLoggerConfig struct {
	// ServiceName is the name of the service (from K_SERVICE in Cloud Run)
	ServiceName string
	// ServiceVersion is the version/revision (from K_REVISION in Cloud Run)
	ServiceVersion string
	// LogLevel is the logging level (debug, info, warn, error)
	LogLevel string
	// Output is the writer for log output (defaults to os.Stderr)
	Output io.Writer
}

// DefaultServiceName is the fallback service name when not provided
const DefaultServiceName = "vibexp-api"

// DefaultServiceVersion is the fallback version when not provided
const DefaultServiceVersion = "local"

// errorReportingType is the Cloud Error Reporting discriminator value. When a
// log entry includes this `@type` field (and a sibling serviceContext), GCP
// auto-groups it as a ReportedErrorEvent and surfaces it in the Error
// Reporting console.
const errorReportingType = "type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent"

// ServiceHook is a logrus hook that adds service metadata to all log entries.
// For entries at severity ERROR or above it also injects the Cloud Error
// Reporting discriminator (`@type` + `serviceContext`) and expands a wrapped
// `error` field with `%+v` so any attached stack frames flow into `message`.
type ServiceHook struct {
	serviceName    string
	serviceVersion string
}

// NewServiceHook creates a new ServiceHook with the provided service metadata
func NewServiceHook(serviceName, serviceVersion string) *ServiceHook {
	if serviceName == "" {
		serviceName = DefaultServiceName
	}

	if serviceVersion == "" {
		serviceVersion = DefaultServiceVersion
	}

	return &ServiceHook{
		serviceName:    serviceName,
		serviceVersion: serviceVersion,
	}
}

// Levels returns the log levels this hook applies to
func (h *ServiceHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire is called for each log entry to add service metadata. ERROR-and-above
// entries are additionally annotated for Cloud Error Reporting.
func (h *ServiceHook) Fire(entry *logrus.Entry) error {
	// Add service metadata if not already present
	if _, exists := entry.Data["service"]; !exists {
		entry.Data["service"] = h.serviceName
	}

	if _, exists := entry.Data["version"]; !exists {
		entry.Data["version"] = h.serviceVersion
	}

	// Logrus levels are ordered Panic(0) < Fatal < Error < Warn < ... so
	// "ErrorLevel and above" is `level <= ErrorLevel`.
	if entry.Level <= logrus.ErrorLevel {
		h.annotateForErrorReporting(entry)
	}

	return nil
}

// annotateForErrorReporting injects the Cloud Error Reporting discriminator
// and serviceContext, and expands any wrapped error onto the message so its
// stack frames (when present, e.g. pkg/errors) survive into the parsed event.
func (h *ServiceHook) annotateForErrorReporting(entry *logrus.Entry) {
	if _, exists := entry.Data["@type"]; !exists {
		entry.Data["@type"] = errorReportingType
	}

	if _, exists := entry.Data["serviceContext"]; !exists {
		entry.Data["serviceContext"] = map[string]string{
			"service": h.serviceName,
			"version": h.serviceVersion,
		}
	}

	// If logrus.WithError(err) was used, the wrapped error is in
	// Data[logrus.ErrorKey] (defaults to "error"; we use the package var so a
	// reassignment elsewhere can't silently break this hook). Format with %+v
	// so error types that carry frames (pkg/errors, cockroachdb errors) flow
	// them into the message field. For plain errors, %+v reduces to %v, so
	// this is a no-op in the worst case.
	if rawErr, ok := entry.Data[logrus.ErrorKey]; ok {
		if err, ok := rawErr.(error); ok && err != nil {
			formatted := fmt.Sprintf("%+v", err)
			if entry.Message == "" {
				entry.Message = formatted
			} else {
				entry.Message = entry.Message + ": " + formatted
			}
		}
	}
}

// CloudFormatter is a custom formatter for Google Cloud Logging
type CloudFormatter struct {
	TimestampFormat string
}

// NewCloudFormatter creates a Cloud Logging optimized JSON formatter
func NewCloudFormatter() *CloudFormatter {
	return &CloudFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
	}
}

// logrusToCloudSeverity converts logrus level to Cloud Logging severity
func logrusToCloudSeverity(level logrus.Level) string {
	// Cloud Logging expects: DEFAULT, DEBUG, INFO, NOTICE, WARNING, ERROR, CRITICAL, ALERT, EMERGENCY
	switch level {
	case logrus.TraceLevel:
		return "DEBUG"
	case logrus.DebugLevel:
		return "DEBUG"
	case logrus.InfoLevel:
		return "INFO"
	case logrus.WarnLevel:
		return "WARNING"
	case logrus.ErrorLevel:
		return "ERROR"
	case logrus.FatalLevel:
		return "CRITICAL"
	case logrus.PanicLevel:
		return "EMERGENCY"
	default:
		return "DEFAULT"
	}
}

// Format formats the log entry for Cloud Logging
func (f *CloudFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// Create a new map for the output
	data := make(logrus.Fields, len(entry.Data)+3)

	// Copy all existing fields
	for k, v := range entry.Data {
		data[k] = v
	}

	// Add Cloud Logging standard fields
	data["timestamp"] = entry.Time.Format(f.TimestampFormat)
	data["severity"] = logrusToCloudSeverity(entry.Level)
	data["message"] = entry.Message

	// Use JSON formatter for the actual encoding
	serialized, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return append(serialized, '\n'), nil
}

// NewCloudLogger creates a new logger optimized for Google Cloud Logging
// Config values can be empty strings, in which case defaults will be used
func NewCloudLogger(cfg CloudLoggerConfig) *logrus.Logger {
	logger := logrus.New()

	// Set Cloud Logging optimized formatter
	logger.SetFormatter(NewCloudFormatter())

	// Output to stderr (Cloud Run captures stderr for logging)
	if cfg.Output != nil {
		logger.SetOutput(cfg.Output)
	} else {
		logger.SetOutput(os.Stderr)
	}

	// Set log level from config or default to Info
	switch cfg.LogLevel {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "warn", "warning":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	// Add service hook to inject metadata into all log entries
	logger.AddHook(NewServiceHook(cfg.ServiceName, cfg.ServiceVersion))

	return logger
}
