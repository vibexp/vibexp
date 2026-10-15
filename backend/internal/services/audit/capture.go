package audit

import "context"

// ClientInfoCapturer says whether new audit events store the client's IP address
// and user agent (epic #1510, decision 9). The recorder consults it off the
// request path, once per event.
type ClientInfoCapturer interface {
	// CaptureClientInfo reports whether to store IP and user agent. An
	// implementation that cannot read its setting decides for itself; it has
	// no way to fail the event.
	CaptureClientInfo(ctx context.Context) bool
}

// AlwaysCapture is the ClientInfoCapturer the recorder falls back to when
// RecorderDeps.Capture is nil: capture is on, the documented default. Production
// wiring binds the instance's capture_ip_ua setting instead (#1512).
type AlwaysCapture struct{}

var _ ClientInfoCapturer = AlwaysCapture{}

// CaptureClientInfo always reports true.
func (AlwaysCapture) CaptureClientInfo(context.Context) bool { return true }
