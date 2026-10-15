package audit

import "context"

// CaptureSettings says whether new audit events store the client's IP address
// and user agent (epic #1510, decision 9). The recorder consults it off the
// request path, once per event.
type CaptureSettings interface {
	// CaptureClientInfo reports whether to store IP and user agent. An
	// implementation that cannot read its setting decides for itself; it has
	// no way to fail the event.
	CaptureClientInfo(ctx context.Context) bool
}

// AlwaysCapture is the CaptureSettings in force until the instance audit
// settings exist (#1512): capture is on, the documented default.
type AlwaysCapture struct{}

var _ CaptureSettings = AlwaysCapture{}

// CaptureClientInfo always reports true.
func (AlwaysCapture) CaptureClientInfo(context.Context) bool { return true }
