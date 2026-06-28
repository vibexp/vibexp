package oauthserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"

	gojose "github.com/go-jose/go-jose/v3"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	signingKeyBits = 2048
	signingKeyAlg  = "RS256"
	signingKeyUse  = "sig"
	kidByteLen     = 16
	pemTypeRSAPriv = "RSA PRIVATE KEY"
	rotationLeeway = time.Minute // avoid double-rotating on near-boundary checks
)

// KeyManager owns the DB-backed JWT signing keys. The active key signs new
// access tokens; retired keys stay published in the JWKS until their tokens
// expire. The active private key is cached to avoid a decrypt on every sign.
type KeyManager struct {
	repo             repositories.OAuthSigningKeyRepository
	encKey           []byte
	rotationInterval time.Duration

	mu        sync.RWMutex
	activeJWK *gojose.JSONWebKey // private JWK (Key is *rsa.PrivateKey), KeyID = kid
	activeAge time.Time
}

// NewKeyManager constructs a KeyManager. encKey must be the 32-byte app
// encryption key used to seal private keys at rest.
func NewKeyManager(
	repo repositories.OAuthSigningKeyRepository, encKey []byte, rotationInterval time.Duration,
) *KeyManager {
	return &KeyManager{repo: repo, encKey: encKey, rotationInterval: rotationInterval}
}

// EnsureActiveKey guarantees an active signing key exists, generating the first
// one on a fresh deployment. Safe to call on every startup.
func (m *KeyManager) EnsureActiveKey(ctx context.Context) error {
	_, err := m.loadActive(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, repositories.ErrOAuthSigningKeyNotFound) {
		return err
	}
	if genErr := m.generateAndActivate(ctx); genErr != nil {
		// A concurrent instance may have won a cold-start race and activated its
		// key first, so ours hits the single-active-key constraint. Fall back to
		// whatever key is now active rather than failing to boot.
		if _, loadErr := m.loadActive(ctx); loadErr == nil {
			return nil
		}
		return genErr
	}
	return nil
}

// PrivateKeyGetter returns the fosite signer key getter. It yields the active
// private JWK so signed JWTs carry the matching `kid` header.
func (m *KeyManager) PrivateKeyGetter() func(context.Context) (interface{}, error) {
	return func(ctx context.Context) (interface{}, error) {
		return m.loadActive(ctx)
	}
}

// PublicJWKS returns the public JSON Web Key Set (active + retired keys).
func (m *KeyManager) PublicJWKS(ctx context.Context) (*gojose.JSONWebKeySet, error) {
	keys, err := m.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	set := &gojose.JSONWebKeySet{}
	for _, k := range keys {
		var jwk gojose.JSONWebKey
		if uerr := jwk.UnmarshalJSON(k.PublicJWK); uerr != nil {
			return nil, fmt.Errorf("oauthserver: parse public jwk %s: %w", k.KID, uerr)
		}
		set.Keys = append(set.Keys, jwk)
	}
	return set, nil
}

// MaybeRotate rotates the active key when it is older than the rotation interval.
// It is a no-op when rotation is not yet due. Rotation is coordinated across
// instances with a Postgres advisory lock so only one instance mints a new key
// per interval; instances that lose the race drop their cache and pick up the
// winner's key on the next sign.
func (m *KeyManager) MaybeRotate(ctx context.Context) (err error) {
	if _, err = m.loadActive(ctx); err != nil {
		return err
	}
	m.mu.RLock()
	age := m.activeAge
	m.mu.RUnlock()
	if time.Since(age) < m.rotationInterval-rotationLeeway {
		return nil
	}

	acquired, release, lockErr := m.repo.TryAdvisoryLock(ctx)
	if lockErr != nil {
		return lockErr
	}
	if !acquired {
		// A peer is rotating; drop our cache so the next loadActive reads the new
		// active key from the DB rather than signing with a soon-to-retire key.
		m.invalidateCache()
		return nil
	}
	defer func() {
		if rerr := release(); rerr != nil {
			err = errors.Join(err, rerr)
		}
	}()

	// Re-check against the DB under the lock: a peer may have rotated between our
	// cached check and acquiring the lock.
	row, getErr := m.repo.GetActive(ctx)
	if getErr != nil {
		return getErr
	}
	if time.Since(row.CreatedAt) < m.rotationInterval-rotationLeeway {
		return m.cacheActive(row)
	}
	return m.generateAndActivate(ctx)
}

// PruneRetired removes retired signing keys old enough that no live token can
// still reference them: a key retired more than the refresh-token TTL ago cannot
// have signed any token that is still valid. Returns the number of keys removed.
func (m *KeyManager) PruneRetired(ctx context.Context, refreshTokenTTL time.Duration) (int64, error) {
	return m.repo.DeleteRetiredBefore(ctx, time.Now().Add(-refreshTokenTTL))
}

// invalidateCache forgets the cached active key so the next loadActive re-reads
// it from the DB.
func (m *KeyManager) invalidateCache() {
	m.mu.Lock()
	m.activeJWK = nil
	m.mu.Unlock()
}

// cacheActive refreshes the cached active key from a freshly-read row.
func (m *KeyManager) cacheActive(row *models.OAuthSigningKey) error {
	jwk, err := m.privateJWKFromRow(row)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.activeJWK = jwk
	m.activeAge = row.CreatedAt
	m.mu.Unlock()
	return nil
}

// loadActive returns the cached active private JWK, loading and decrypting it
// from the DB on a cache miss.
func (m *KeyManager) loadActive(ctx context.Context) (*gojose.JSONWebKey, error) {
	m.mu.RLock()
	cached := m.activeJWK
	m.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	row, err := m.repo.GetActive(ctx)
	if err != nil {
		return nil, err
	}
	jwk, err := m.privateJWKFromRow(row)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.activeJWK = jwk
	m.activeAge = row.CreatedAt
	m.mu.Unlock()
	return jwk, nil
}

func (m *KeyManager) privateJWKFromRow(row *models.OAuthSigningKey) (*gojose.JSONWebKey, error) {
	pemBytes, err := open(m.encKey, row.PrivateKeyEncrypted)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("oauthserver: signing key %s has no PEM block", row.KID)
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("oauthserver: parse signing key %s: %w", row.KID, err)
	}
	return &gojose.JSONWebKey{
		Key:       priv,
		KeyID:     row.KID,
		Algorithm: row.Algorithm,
		Use:       signingKeyUse,
	}, nil
}

// generateAndActivate creates a new RSA key, persists it, promotes it to active,
// and refreshes the cache.
func (m *KeyManager) generateAndActivate(ctx context.Context) error {
	row, err := newSigningKeyRow(m.encKey)
	if err != nil {
		return err
	}
	if err = m.repo.Create(ctx, row); err != nil {
		return err
	}
	if err = m.repo.Activate(ctx, row.KID); err != nil {
		return err
	}

	jwk, err := m.privateJWKFromRow(row)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.activeJWK = jwk
	m.activeAge = time.Now()
	m.mu.Unlock()
	return nil
}

// newSigningKeyRow generates an RSA keypair and returns a persistable, inactive
// signing-key row with the private key sealed at rest.
func newSigningKeyRow(encKey []byte) (*models.OAuthSigningKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, signingKeyBits)
	if err != nil {
		return nil, fmt.Errorf("oauthserver: generate rsa key: %w", err)
	}
	kidBytes := make([]byte, kidByteLen)
	if _, err = rand.Read(kidBytes); err != nil {
		return nil, fmt.Errorf("oauthserver: generate kid: %w", err)
	}
	kid := hex.EncodeToString(kidBytes)

	pubJWK := gojose.JSONWebKey{Key: &priv.PublicKey, KeyID: kid, Algorithm: signingKeyAlg, Use: signingKeyUse}
	pubJSON, err := pubJWK.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("oauthserver: marshal public jwk: %w", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: pemTypeRSAPriv, Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	sealed, err := seal(encKey, pemBytes)
	if err != nil {
		return nil, err
	}

	return &models.OAuthSigningKey{
		KID:                 kid,
		Algorithm:           signingKeyAlg,
		PrivateKeyEncrypted: sealed,
		PublicJWK:           pubJSON,
		Active:              false,
	}, nil
}

// publicJWKSJSON marshals the JWKS for the HTTP endpoint.
func publicJWKSJSON(set *gojose.JSONWebKeySet) ([]byte, error) {
	return json.Marshal(set)
}
