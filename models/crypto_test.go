package models

import (
	"autosleep/constants"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"io"
	"testing"
)

func TestEncryptRoundTrip(t *testing.T) {
	ct := Encrypt("heroku-token-0123456789")
	if ct[0] != envelopeV2 {
		t.Fatalf("expected v2 envelope, got 0x%x", ct[0])
	}
	pt, err := Decrypt(ct)
	if err != nil || pt != "heroku-token-0123456789" {
		t.Fatalf("Decrypt = %q, %v", pt, err)
	}
	if string(Encrypt("x")) == string(Encrypt("x")) {
		t.Fatal("ciphertexts must differ (random nonce)")
	}
}

// Rows written by earlier versions must stay readable.
func TestDecryptLegacyV1(t *testing.T) {
	sum := md5.Sum([]byte(constants.Passphrase))
	block, _ := aes.NewCipher([]byte(hex.EncodeToString(sum[:])))
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	legacy := gcm.Seal(nonce, nonce, []byte("old-api-key"), nil)

	pt, err := Decrypt(legacy)
	if err != nil || pt != "old-api-key" {
		t.Fatalf("Decrypt v1 = %q, %v", pt, err)
	}
}

func TestDecryptRejectsTamperedAndEmpty(t *testing.T) {
	ct := Encrypt("secret")
	ct[len(ct)-1] ^= 0xff
	if _, err := Decrypt(ct); err != ErrDecrypt {
		t.Fatalf("tampered: err = %v", err)
	}
	if _, err := Decrypt(nil); err != ErrDecrypt {
		t.Fatalf("empty: err = %v", err)
	}
}
