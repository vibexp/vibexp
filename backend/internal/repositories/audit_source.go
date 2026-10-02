package repositories

import "context"

// AuditSourceCLI marks an audited instance settings write made by the
// break-glass `vibexp admin auth` commands (#1237) rather than through the API.
const AuditSourceCLI = "cli"

// auditSourceKey is the context key WithAuditSource stores under. It is an
// unexported type so no other package can set or shadow the value.
type auditSourceKey struct{}

// WithAuditSource returns a context whose audited instance auth settings writes
// record source in their audit snapshot. It travels in the context so the
// repositories' signatures stay the ones the API uses. An empty source returns
// ctx unchanged.
func WithAuditSource(ctx context.Context, source string) context.Context {
	if source == "" {
		return ctx
	}
	return context.WithValue(ctx, auditSourceKey{}, source)
}

// AuditSourceFromContext returns the source set by WithAuditSource, or "" when
// there is none (a write made through the API).
func AuditSourceFromContext(ctx context.Context) string {
	source, _ := ctx.Value(auditSourceKey{}).(string)
	return source
}
