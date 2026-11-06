package infrastructure

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const (
	argonMemoryKiB   uint32 = 19 * 1024
	argonIterations  uint32 = 2
	argonParallelism uint8  = 1
	saltLength              = 16
	keyLength               = 32
)

var errInvalidPasswordHash = errors.New("invalid password hash")

// PasswordHasher uses Argon2id for new hashes. The bounded semaphore prevents
// password endpoints from turning unbounded requests into unbounded memory use.
type PasswordHasher struct {
	work      chan struct{}
	dummy     sync.Once
	dummyHash string
}

// NewPasswordHasher creates a hasher with a finite work queue. The argument is
// retained as a single setting so local callers do not need to know Argon2
// parameters; it is clamped to a small safe range.
func NewPasswordHasher(workers int) *PasswordHasher {
	if workers < 1 {
		workers = 1
	}
	if workers > 8 {
		workers = 8
	}
	return &PasswordHasher{work: make(chan struct{}, workers)}
}

func (h *PasswordHasher) Hash(password string) (string, error) {
	if err := h.acquire(); err != nil {
		return "", err
	}
	defer h.release()
	return hashArgon2id(password)
}

func (h *PasswordHasher) Compare(encoded, password string) error {
	if strings.HasPrefix(encoded, "$2") {
		if len([]byte(password)) > 72 {
			return errInvalidPasswordHash
		}
		return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
	}
	params, salt, expected, err := parseArgon2id(encoded)
	if err != nil {
		return errInvalidPasswordHash
	}
	if err := h.acquire(); err != nil {
		return err
	}
	defer h.release()
	actual := argon2.IDKey([]byte(password), salt, params.memory, params.iterations, params.parallelism, uint32(len(expected)))
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return errInvalidPasswordHash
	}
	return nil
}

func (h *PasswordHasher) NeedsRehash(encoded string) bool {
	params, _, _, err := parseArgon2id(encoded)
	return err != nil || params.memory != argonMemoryKiB || params.iterations != argonIterations || params.parallelism != argonParallelism
}

// DummyHash is generated with the same working parameters as a real password
// and is used for unknown accounts.
func (h *PasswordHasher) DummyHash() string {
	h.dummy.Do(func() { h.dummyHash, _ = h.Hash("not-a-user-password") })
	return h.dummyHash
}

func (h *PasswordHasher) acquire() error {
	select {
	case h.work <- struct{}{}:
		return nil
	default:
		return errors.New("password hash queue is full")
	}
}
func (h *PasswordHasher) release() { <-h.work }

func hashArgon2id(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonMemoryKiB, argonIterations, argonParallelism, keyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemoryKiB, argonIterations, argonParallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

type argonParams struct {
	memory, iterations uint32
	parallelism        uint8
}

func parseArgon2id(encoded string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return argonParams{}, nil, nil, errInvalidPasswordHash
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil || memory < 8*1024 || memory > 64*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 4 {
		return argonParams{}, nil, nil, errInvalidPasswordHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return argonParams{}, nil, nil, errInvalidPasswordHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return argonParams{}, nil, nil, errInvalidPasswordHash
	}
	return argonParams{memory: memory, iterations: iterations, parallelism: parallelism}, salt, key, nil
}
