package idp

import "strings"

// defaultDisplayNames maps a canonical provider name to the human label the
// login UI shows for it. The login screen and the boot-time import of the
// legacy config.yaml providers (#1232) share it, so an imported provider keeps
// the label it had.
var defaultDisplayNames = map[ProviderName]string{
	ProviderGoogle: "Google",
	ProviderGitHub: "GitHub",
	ProviderOIDC:   "Single Sign-On",
}

// DefaultDisplayName returns the UI label for a canonical provider name,
// title-casing unknown names as a sensible default so a newly-added or generic
// provider still renders sensibly.
func DefaultDisplayName(name ProviderName) string {
	if label, ok := defaultDisplayNames[name]; ok {
		return label
	}
	if name == "" {
		return ""
	}
	return strings.ToUpper(string(name[:1])) + string(name[1:])
}
