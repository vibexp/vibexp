package oauthserver

import "net/http"

// Token handles POST /oauth2/token: the authorization_code and refresh_token
// grants. fosite enforces PKCE verification, client/redirect checks, refresh
// rotation, and reuse detection; access tokens are minted as JWTs bound to the
// resource audience.
func (s *Service) Token(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ar, err := s.provider.NewAccessRequest(ctx, r, newEmptySession())
	if err != nil {
		s.provider.WriteAccessError(ctx, w, ar, err)
		return
	}
	resp, err := s.provider.NewAccessResponse(ctx, ar)
	if err != nil {
		s.provider.WriteAccessError(ctx, w, ar, err)
		return
	}
	s.provider.WriteAccessResponse(ctx, w, ar, resp)
}
