package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provider clients share one SSRF-guarded transport per guard (#1082): a
// transport per construction made the pool settings dead config and leaked an
// idle connection per request for IdleConnTimeout.

func TestProviderHTTPClient_SameGuardSharesTransportKeepsOwnTimeout(t *testing.T) {
	guard := &ssrfGuard{}

	probe := newProviderHTTPClient(guard, validateModelProviderTimeout)
	completion := newProviderHTTPClient(guard, completionTimeout)

	require.NotNil(t, probe.Transport)
	assert.Same(t, probe.Transport, completion.Transport,
		"clients from one guard must share a single transport")
	assert.Equal(t, validateModelProviderTimeout, probe.Timeout)
	assert.Equal(t, completionTimeout, completion.Timeout)
	assert.NotEqual(t, probe.Timeout, completion.Timeout,
		"each call site keeps its own timeout over the shared transport")
}

func TestProviderHTTPClient_NilGuardSharesDefaultTransport(t *testing.T) {
	first := newProviderHTTPClient(nil, time.Second)
	second := newProviderHTTPClient(nil, 2*time.Second)

	assert.Same(t, first.Transport, second.Transport)
	assert.Same(t, defaultSSRFGuard.sharedProviderTransport(), first.Transport,
		"a nil guard must reuse the production guard's transport")
}

func TestProviderHTTPClient_DistinctPoliciesNeverShareTransport(t *testing.T) {
	_, allowed, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)

	strict := newProviderHTTPClient(&ssrfGuard{}, time.Second)
	allowlisted := newProviderHTTPClient(&ssrfGuard{allowedCIDRs: []*net.IPNet{allowed}}, time.Second)
	localDev := newProviderHTTPClient(&ssrfGuard{allowPrivate: true}, time.Second)

	assert.NotSame(t, strict.Transport, allowlisted.Transport)
	assert.NotSame(t, strict.Transport, localDev.Transport)
	assert.NotSame(t, allowlisted.Transport, localDev.Transport)
}

// TestProviderHTTPClient_ReusesConnectionAcrossConstructions is the point of
// the change: two separately-constructed provider clients reach the same host
// over one pooled connection instead of dialing twice.
func TestProviderHTTPClient_ReusesConnectionAcrossConstructions(t *testing.T) {
	var newConns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	guard := &ssrfGuard{allowPrivate: true}
	for i := range 2 {
		client := newProviderHTTPClient(guard, 5*time.Second)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, http.NoBody)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err, "request %d", i)
		require.NoError(t, resp.Body.Close())
	}

	assert.Equal(t, int32(1), newConns.Load(),
		"the second provider client must reuse the first one's pooled connection")
}
