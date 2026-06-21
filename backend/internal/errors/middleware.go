package errors

import (
	"net/http"
	"runtime/debug"

	"github.com/sirupsen/logrus"
)

// RecoveryMiddleware recovers from panics and returns a proper error response
func RecoveryMiddleware(logger *logrus.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rvr := recover(); rvr != nil {
					logger.WithFields(logrus.Fields{
						"service":    "vibexp-api",
						"middleware": "RecoveryMiddleware",
						"panic":      rvr,
						"stack":      string(debug.Stack()),
						"path":       r.URL.Path,
						"method":     r.Method,
					}).Error("Panic recovered")

					apiErr := NewInternalError("An unexpected error occurred")
					WriteJSONError(w, r, apiErr)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
