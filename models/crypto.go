package models

import (
	"autosleep/constants"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Stored secrets (Heroku API keys and OAuth tokens) use one of two
// AES-256-GCM envelopes:
//
//	v1 (legacy, read-only): [12-byte nonce][ciphertext+tag]
//	    key = hex(md5(PASSPHRASE))
//	v2 (current):           0x02 [32-byte salt][12-byte nonce][ciphertext+tag]
//	    key = argon2id(PASSPHRASE, salt)
//
// Encrypt always writes v2; Decrypt reads both, so existing rows keep
// working and are upgraded whenever they are rewritten (e.g. on token
// refresh or when an API key is updated).
const (
	envelopeV2 byte = 0x02
	saltLen         = 32
	nonceLen        = 12

	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024 // KiB, i.e. 64 MiB
	argonThreads uint8  = 2
)

// ErrDecrypt usually means PASSPHRASE changed since the secret was stored.
var ErrDecrypt = errors.New("could not decrypt stored secret (was PASSPHRASE changed?)")

// Argon2id with 64 MiB is deliberately expensive, so derived keys are cached
// by salt and derived one at a time; a small dyno running many concurrent
// jobs must not allocate 64 MiB per job. Encrypt reuses one salt per
// process (nonces stay random), so it derives a key only once.
var (
	keyMu       sync.Mutex
	keyCache    = map[string]cipher.AEAD{}
	encSaltMu   sync.Mutex
	encryptSalt []byte // salt used by Encrypt in this process
)

func aeadV2(salt []byte) cipher.AEAD {
	keyMu.Lock()
	defer keyMu.Unlock()
	if gcm, ok := keyCache[string(salt)]; ok {
		return gcm
	}
	key := argon2.IDKey([]byte(constants.Passphrase), salt, argonTime, argonMemory, argonThreads, 32)
	gcm := newGCM(key)
	keyCache[string(salt)] = gcm
	return gcm
}

// aeadV1 reproduces the legacy key derivation, kept only to read old rows.
func aeadV1() cipher.AEAD {
	sum := md5.Sum([]byte(constants.Passphrase)) // #nosec G401 -- legacy read path only
	return newGCM([]byte(hex.EncodeToString(sum[:])))
}

func newGCM(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}
	return gcm
}

func processSalt() []byte {
	encSaltMu.Lock()
	defer encSaltMu.Unlock()
	if encryptSalt == nil {
		salt := make([]byte, saltLen)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			panic(err.Error())
		}
		encryptSalt = salt
	}
	return encryptSalt
}

// Encrypt seals data in the v2 envelope.
func Encrypt(data string) []byte {
	salt := processSalt()
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err.Error())
	}
	out := make([]byte, 0, 1+saltLen+nonceLen+len(data)+16)
	out = append(out, envelopeV2)
	out = append(out, salt...)
	out = append(out, nonce...)
	return aeadV2(salt).Seal(out, nonce, []byte(data), nil)
}

// Decrypt opens a v2 or legacy v1 envelope.
func Decrypt(data []byte) (string, error) {
	if len(data) > 1+saltLen+nonceLen && data[0] == envelopeV2 {
		salt := data[1 : 1+saltLen]
		nonce := data[1+saltLen : 1+saltLen+nonceLen]
		if pt, err := aeadV2(salt).Open(nil, nonce, data[1+saltLen+nonceLen:], nil); err == nil {
			return string(pt), nil
		}
		// A v1 ciphertext can start with 0x02 by chance; try it below.
	}
	if len(data) < nonceLen {
		return "", ErrDecrypt
	}
	pt, err := aeadV1().Open(nil, data[:nonceLen], data[nonceLen:], nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(pt), nil
}
