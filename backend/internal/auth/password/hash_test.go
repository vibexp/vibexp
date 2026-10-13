package password

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cheapParams keeps the suite fast: a real DefaultParams hash costs 19 MiB.
var cheapParams = Params{Memory: 64, Time: 1, Threads: 1, SaltLength: 16, KeyLength: 32}

const testPassword = "correct horse battery staple"

var ctx = context.Background()

func TestHasher_HashAndVerify(t *testing.T) {
	h := NewHasher(cheapParams)

	encoded, err := h.Hash(ctx, testPassword)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encoded, "$argon2id$v=19$m=64,t=1,p=1$"), encoded)
	assert.NotContains(t, encoded, testPassword)

	ok, needsRehash, err := h.Verify(ctx, encoded, testPassword)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, needsRehash)

	for name, wrong := range map[string]string{
		"different password": "correct horse battery stapl3",
		"empty":              "",
		"trailing space":     testPassword + " ",
		"different case":     strings.ToUpper(testPassword),
	} {
		t.Run(name, func(t *testing.T) {
			ok, needsRehash, err := h.Verify(ctx, encoded, wrong)
			require.NoError(t, err, "a wrong password is not an error")
			assert.False(t, ok)
			assert.False(t, needsRehash)
		})
	}
}

func TestHasher_DerivationsCountsRealWork(t *testing.T) {
	h := NewHasher(cheapParams)
	require.Zero(t, h.Derivations(), "building a hasher is not a sign-in's work")

	encoded, err := h.Hash(ctx, testPassword)
	require.NoError(t, err)
	_, _, err = h.Verify(ctx, encoded, testPassword)
	require.NoError(t, err)
	require.NoError(t, h.DummyVerify(ctx, testPassword))
	assert.Equal(t, int64(3), h.Derivations())

	_, _, err = h.Verify(ctx, "not-a-hash", testPassword)
	require.ErrorIs(t, err, ErrMalformedHash)
	assert.Equal(t, int64(3), h.Derivations(), "an unreadable hash costs no computation")
}

// Building a Hasher computes nothing. A server builds one at start-up and the
// test suite builds thousands of servers; an eager derivation here once made
// the server package's race-enabled run exceed CI's time limit.
func TestNewHasher_ComputesNothing(t *testing.T) {
	h := NewHasher(DefaultParams)

	assert.Zero(t, h.Derivations())
	assert.Nil(t, h.dummyKey, "the dummy derivation is made on first use, not at construction")
}

func TestHasher_HashUsesAFreshSalt(t *testing.T) {
	h := NewHasher(cheapParams)

	first, err := h.Hash(ctx, testPassword)
	require.NoError(t, err)
	second, err := h.Hash(ctx, testPassword)
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "two hashes of one password must differ")
}

// A hash made with other parameters still verifies, with the parameters it
// names, and is reported for replacement.
func TestHasher_VerifyReportsOutdatedParameters(t *testing.T) {
	old := NewHasher(Params{Memory: 32, Time: 1, Threads: 1, SaltLength: 16, KeyLength: 32})
	current := NewHasher(cheapParams)
	encoded, err := old.Hash(ctx, testPassword)
	require.NoError(t, err)

	ok, needsRehash, err := current.Verify(ctx, encoded, testPassword)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, needsRehash)

	ok, needsRehash, err = current.Verify(ctx, encoded, "not the password")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.False(t, needsRehash, "a wrong password is never a reason to rehash")
}

func TestHasher_VerifyRejectsMalformedHashes(t *testing.T) {
	h := NewHasher(cheapParams)
	valid, err := h.Hash(ctx, testPassword)
	require.NoError(t, err)
	parts := strings.Split(valid, "$") // "", argon2id, v=19, m=..,t=..,p=.., salt, key
	rebuild := func(index int, value string) string {
		changed := append([]string(nil), parts...)
		changed[index] = value
		return strings.Join(changed, "$")
	}

	cases := map[string]string{
		"empty":                     "",
		"not a hash":                "plaintext-password",
		"bcrypt":                    "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
		"argon2i":                   rebuild(1, "argon2i"),
		"other version":             rebuild(2, "v=16"),
		"missing a cost field":      rebuild(3, "m=64,t=1"),
		"cost fields out of order":  rebuild(3, "t=1,m=64,p=1"),
		"zero memory":               rebuild(3, "m=0,t=1,p=1"),
		"zero time":                 rebuild(3, "m=64,t=0,p=1"),
		"zero threads":              rebuild(3, "m=64,t=1,p=0"),
		"memory below 8 per thread": rebuild(3, "m=15,t=1,p=2"),
		"memory above the bound":    rebuild(3, "m=4294967295,t=1,p=1"),
		"time above the bound":      rebuild(3, "m=64,t=1000,p=1"),
		"threads above the bound":   rebuild(3, "m=64,t=1,p=255"),
		"negative memory":           rebuild(3, "m=-64,t=1,p=1"),
		"signed memory":             rebuild(3, "m=+64,t=1,p=1"),
		"memory overflows":          rebuild(3, "m=99999999999,t=1,p=1"),
		"salt not base64":           rebuild(4, "!!!!"),
		"salt too short":            rebuild(4, "YWJj"),
		"key not base64":            rebuild(5, "!!!!"),
		"key too short":             rebuild(5, "YWJj"),
		"extra field":               valid + "$extra",
		"missing key":               strings.Join(parts[:5], "$"),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			ok, needsRehash, err := h.Verify(ctx, encoded, testPassword)

			require.ErrorIs(t, err, ErrMalformedHash)
			assert.False(t, ok)
			assert.False(t, needsRehash)
			if encoded != "" {
				assert.NotContains(t, err.Error(), encoded, "the error must not carry the hash")
			}
		})
	}
}

func TestHasher_DummyVerifyRunsWithoutAHash(t *testing.T) {
	h := NewHasher(cheapParams)

	// It has no result to assert; it must simply complete, repeatedly and
	// concurrently.
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { assert.NoError(t, h.DummyVerify(ctx, testPassword)) })
	}
	wg.Wait()

	// What it compares against is a real derivation under the hasher's
	// parameters, made once on first use.
	ok, _, err := h.Verify(ctx, encode(h.params, h.dummySalt, h.dummyKey), "dummy-password-for-timing")
	require.NoError(t, err)
	assert.True(t, ok)
}

// A caller whose context ended while every slot was busy gives up without
// computing anything, so a backlog of abandoned requests is dropped instead of
// being worked off.
func TestHasher_GivesUpItsPlaceWhenTheContextEnds(t *testing.T) {
	h := NewHasher(cheapParams)
	encoded, err := h.Hash(ctx, testPassword)
	require.NoError(t, err)
	// Occupy every slot, as a flood of in-flight computations would.
	for range DefaultMaxConcurrent {
		h.slots <- struct{}{}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	_, hashErr := h.Hash(cancelled, testPassword)
	ok, needsRehash, verifyErr := h.Verify(cancelled, encoded, testPassword)
	dummyErr := h.DummyVerify(cancelled, testPassword)

	for _, got := range []error{hashErr, verifyErr, dummyErr} {
		require.ErrorIs(t, got, context.Canceled)
		assert.NotContains(t, got.Error(), testPassword)
	}
	assert.False(t, ok, "an abandoned check never reports a match")
	assert.False(t, needsRehash)
	assert.Len(t, h.slots, DefaultMaxConcurrent, "no slot was taken or released by the abandoned calls")
	assert.Equal(t, int64(1), h.Derivations(), "only the one Hash before the flood computed anything")
}

// An already-abandoned request computes nothing even when a slot is free.
func TestHasher_AbandonedRequestNeverComputesEvenWithAFreeSlot(t *testing.T) {
	h := NewHasher(cheapParams)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	for range 50 {
		require.ErrorIs(t, h.DummyVerify(cancelled, testPassword), context.Canceled)
	}

	assert.Zero(t, h.Derivations())
}

// More callers than slots must all finish: the cap queues work, it does not
// drop it.
func TestHasher_ConcurrencyCapQueues(t *testing.T) {
	h := NewHasher(cheapParams)
	encoded, err := h.Hash(ctx, testPassword)
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]bool, DefaultMaxConcurrent*4)
	for i := range results {
		wg.Go(func() {
			ok, _, verifyErr := h.Verify(ctx, encoded, testPassword)
			results[i] = ok && verifyErr == nil
		})
	}
	wg.Wait()

	for i, ok := range results {
		assert.True(t, ok, "verification %d", i)
	}
}

// The production parameters are the documented baseline and produce a hash the
// same Hasher verifies. This is the one test that pays for a real hash.
func TestDefaultParams(t *testing.T) {
	assert.Equal(t, Params{Memory: 19456, Time: 2, Threads: 1, SaltLength: 16, KeyLength: 32}, DefaultParams)

	h := NewHasher(DefaultParams)
	encoded, err := h.Hash(ctx, testPassword)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$"), encoded)
	ok, needsRehash, err := h.Verify(ctx, encoded, testPassword)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, needsRehash)
}
