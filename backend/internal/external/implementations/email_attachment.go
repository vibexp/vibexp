package implementations

import (
	"mime"
	"path/filepath"
)

// attachmentContentType derives a MIME type from an attachment's filename
// extension, falling back to a generic binary type when the extension is
// unknown or absent.
//
// Why: unlike Mailgun (which infers the type itself), the Postmark and SendGrid
// APIs carry an explicit per-attachment content type — Postmark requires it and
// SendGrid uses it to set the MIME part — so the providers must never send an
// empty value. gomail attachments only expose a filename and raw bytes, so the
// extension is the only signal available.
func attachmentContentType(filename string) string {
	if ct := mime.TypeByExtension(filepath.Ext(filename)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
