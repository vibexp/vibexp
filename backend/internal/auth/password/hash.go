// Package password hashes and verifies account passwords with argon2id and
// holds the password policy every flow that sets a password applies (#1431).
//
// It depends on nothing else in the module: no database, no logging, no HTTP.
// Nothing here ever returns, wraps or formats a password or a hash into an
// error, so an error from this package is always safe to log.
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/argon2"
)

// Params are the argon2id cost parameters. They are encoded into every hash,
// so they can be raised later: an older hash still verifies with the parameters
// it was made with, and Verify reports that it should be replaced.
type Params struct {
	// Memory is the memory cost in KiB.
	Memory uint32
	// Time is the number of passes over the memory.
	Time uint32
	// Threads is the degree of parallelism.
	Threads uint8
	// SaltLength and KeyLength are in bytes.
	SaltLength uint32
	KeyLength  uint32
}

// DefaultParams are the production parameters: the OWASP baseline for argon2id
// (19 MiB, two passes, one thread). One verification costs about 19 MiB and
// tens of milliseconds.
var DefaultParams = Params{Memory: 19 * 1024, Time: 2, Threads: 1, SaltLength: 16, KeyLength: 32}

// Upper bounds on the parameters Verify accepts from a stored hash. A hash is
// data from the database: without bounds, a corrupted or planted row could make
// one login attempt allocate gigabytes or run for minutes.
const (
	maxMemoryKiB  = 1024 * 1024 // 1 GiB
	maxTime       = 16
	maxThreads    = 16
	maxSaltLength = 64
	maxKeyLength  = 128
	minSaltLength = 8
	minKeyLength  = 16
)

// DefaultMaxConcurrent caps how many argon2id computations a Hasher runs at
// once. Each holds Params.Memory for its duration, so the cap bounds the memory
// a burst of sign-in attempts can claim; further attempts wait their turn.
const DefaultMaxConcurrent = 8

const (
	algorithm   = "argon2id"
	hashVersion = argon2.Version
)

// ErrMalformedHash is returned by Verify when the stored hash is not an
// argon2id hash this package can read, or names parameters out of bounds. It
// carries no part of the hash.
var ErrMalformedHash = errors.New("password: stored hash is malformed")

// Hasher hashes and verifies passwords under one set of parameters. It is safe
// for concurrent use. Production code uses NewHasher(DefaultParams); tests pass
// cheap parameters so a suite does not spend a real hash on every case.
//
// Every operation takes a context. A computation waits for one of the Hasher's
// slots, and a caller whose request was cancelled or timed out while waiting
// gives up its place instead of spending the work for nobody: under a flood,
// the backlog of abandoned requests is dropped, not worked off.
type Hasher struct {
	params Params
	slots  chan struct{}

	// dummySalt and dummyKey are a fixed derivation for DummyVerify to compare
	// against, made once, on first use (see dummy).
	dummyOnce sync.Once
	dummySalt []byte
	dummyKey  []byte

	derivations atomic.Int64
}

// NewHasher creates a Hasher that makes hashes with params. It computes
// nothing: a Hasher is built once per server, and every test that builds a
// server builds one, so construction must stay free.
func NewHasher(params Params) *Hasher {
	return &Hasher{params: params, slots: make(chan struct{}, DefaultMaxConcurrent)}
}

// dummy returns the fixed derivation DummyVerify compares against, computing it
// on first use. That one-off computation is outside the slots and is not
// counted by Derivations: it happens once per process, on the first sign-in
// that is checked, which then takes about twice as long as every later one.
// Verify makes it too, although it has no use for it, so that the first
// sign-in of a process is slower by the same amount whether or not the account
// exists.
func (h *Hasher) dummy() (salt, key []byte) {
	h.dummyOnce.Do(func() {
		// The input is fixed and public; only the cost matters.
		h.dummySalt = make([]byte, h.params.SaltLength)
		h.dummyKey = argon2.IDKey([]byte("dummy-password-for-timing"), h.dummySalt,
			h.params.Time, h.params.Memory, h.params.Threads, h.params.KeyLength)
	})
	return h.dummySalt, h.dummyKey
}

// Derivations returns how many argon2id computations the Hasher has performed
// through Hash, Verify and DummyVerify. A call that gave up waiting for a slot,
// or a Verify of a hash it could not read, performed none. It exists so the
// cost of a sign-in can be asserted, and can back a metric.
func (h *Hasher) Derivations() int64 {
	return h.derivations.Load()
}

// Hash returns the encoded argon2id hash of password under a fresh random
// salt, in the PHC string format:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>
//
// with salt and key in unpadded standard base64. The only errors are a failed
// salt read and ctx ending while waiting for a slot.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: generate salt: %w", err)
	}
	key, err := h.derive(ctx, password, salt, h.params)
	if err != nil {
		return "", err
	}
	return encode(h.params, salt, key), nil
}

// Verify reports whether password matches the encoded hash. needsRehash is
// true when it matches but the hash was made with parameters other than this
// Hasher's, so the caller should store a fresh Hash of the same password.
//
// A wrong password is ok == false with a nil error. An error means the check
// could not be made: the stored hash could not be read (ErrMalformedHash), or
// ctx ended while waiting for a slot. It never means the password was wrong.
func (h *Hasher) Verify(ctx context.Context, encoded, password string) (ok, needsRehash bool, err error) {
	params, salt, key, err := decode(encoded)
	if err != nil {
		return false, false, err
	}
	// Not needed here; made so the first check of a process costs the same as
	// the first DummyVerify (see dummy). Skipped for an abandoned request, as
	// DummyVerify skips it.
	if ctx.Err() == nil {
		h.dummy()
	}
	candidate, err := h.derive(ctx, password, salt, params)
	if err != nil {
		return false, false, err
	}
	if subtle.ConstantTimeCompare(candidate, key) != 1 {
		return false, false, nil
	}
	return true, params != h.params, nil
}

// DummyVerify spends the same work as a Verify of a real hash and discards the
// result. A sign-in path that has no hash to check (unknown email, no password
// set, locked account) calls it, so that path takes as long as one that does.
// The only error is ctx ending while waiting for a slot.
func (h *Hasher) DummyVerify(ctx context.Context, password string) error {
	// Checked before the dummy is made, so an abandoned request costs nothing
	// even when it is the first one.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("password: waiting to hash: %w", err)
	}
	salt, key := h.dummy()
	candidate, err := h.derive(ctx, password, salt, h.params)
	if err != nil {
		return err
	}
	// The same comparison Verify performs, with the result thrown away.
	subtle.ConstantTimeCompare(candidate, key)
	return nil
}

// derive runs argon2id, holding one of the Hasher's slots while it does. It
// returns ctx's error, without computing anything, when ctx ends before a slot
// is free.
func (h *Hasher) derive(ctx context.Context, password string, salt []byte, p Params) ([]byte, error) {
	// Checked first: with a slot free and the context already done, select
	// would pick either at random, and an abandoned request must never compute.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("password: waiting to hash: %w", err)
	}
	select {
	case h.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("password: waiting to hash: %w", ctx.Err())
	}
	defer func() { <-h.slots }()
	h.derivations.Add(1)
	return argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLength), nil
}

func encode(p Params, salt, key []byte) string {
	return fmt.Sprintf("$%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		algorithm, hashVersion, p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// decode parses a PHC-format argon2id hash and checks its parameters against
// the bounds above. Every failure is ErrMalformedHash.
func decode(encoded string) (Params, []byte, []byte, error) {
	// "$argon2id$v=19$m=..,t=..,p=..$salt$key" splits into an empty first part
	// and five more.
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != algorithm {
		return Params{}, nil, nil, ErrMalformedHash
	}
	if parts[2] != "v="+strconv.Itoa(hashVersion) {
		return Params{}, nil, nil, ErrMalformedHash
	}
	p, ok := parseCost(parts[3])
	if !ok {
		return Params{}, nil, nil, ErrMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < minSaltLength || len(salt) > maxSaltLength {
		return Params{}, nil, nil, ErrMalformedHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < minKeyLength || len(key) > maxKeyLength {
		return Params{}, nil, nil, ErrMalformedHash
	}
	// The lengths are bounded above, so the conversions cannot overflow.
	p.SaltLength = uint32(len(salt)) // #nosec G115 -- bounded by maxSaltLength
	p.KeyLength = uint32(len(key))   // #nosec G115 -- bounded by maxKeyLength
	return p, salt, key, nil
}

// parseCost parses "m=<memory>,t=<time>,p=<threads>" in exactly that order and
// applies the bounds.
func parseCost(s string) (Params, bool) {
	fields := strings.Split(s, ",")
	if len(fields) != 3 {
		return Params{}, false
	}
	memory, ok1 := parseCostField(fields[0], "m=", maxMemoryKiB)
	time, ok2 := parseCostField(fields[1], "t=", maxTime)
	threads, ok3 := parseCostField(fields[2], "p=", maxThreads)
	if !ok1 || !ok2 || !ok3 {
		return Params{}, false
	}
	// argon2 requires at least 8 KiB per thread; less would panic in the library.
	if memory < 8*threads {
		return Params{}, false
	}
	return Params{Memory: memory, Time: time, Threads: uint8(threads)}, true // #nosec G115 -- bounded by maxThreads
}

// parseCostField parses one "<prefix><n>" field with 1 <= n <= limit, in plain
// decimal digits only.
func parseCostField(field, prefix string, limit uint32) (uint32, bool) {
	digits, found := strings.CutPrefix(field, prefix)
	if !found || digits == "" || len(digits) > 10 {
		return 0, false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(digits, 10, 32)
	if err != nil || n < 1 || n > uint64(limit) {
		return 0, false
	}
	return uint32(n), true
}
