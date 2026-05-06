/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package crypto

import (
	"bytes"
	"testing"
)

func TestDeriveKey(t *testing.T) {
	password := "test-password-123"
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error = %v", err)
	}

	key, err := DeriveKey(password, salt)
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}

	if len(key) != KeySize {
		t.Errorf("DeriveKey() key size = %d, want %d", len(key), KeySize)
	}

	key2, err := DeriveKey(password, salt)
	if err != nil {
		t.Fatalf("DeriveKey() error = %v", err)
	}

	if !bytes.Equal(key, key2) {
		t.Error("DeriveKey() same password and salt should produce same key")
	}
}

func TestDeriveKey_InvalidSaltSize(t *testing.T) {
	_, err := DeriveKey("password", []byte("short"))
	if err == nil {
		t.Error("DeriveKey() should return error for invalid salt size")
	}
}

func TestGenerateSalt(t *testing.T) {
	salt1, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error = %v", err)
	}

	if len(salt1) != SaltSize {
		t.Errorf("GenerateSalt() salt size = %d, want %d", len(salt1), SaltSize)
	}

	salt2, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error = %v", err)
	}

	if bytes.Equal(salt1, salt2) {
		t.Error("GenerateSalt() should generate unique salts")
	}
}

func TestEncrypt_Decrypt(t *testing.T) {
	plaintext := []byte("secret data here")
	key := make([]byte, KeySize)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypt() = %s, want %s", decrypted, plaintext)
	}
}

func TestEncrypt_DifferentCiphertexts(t *testing.T) {
	plaintext := []byte("same text")
	key := make([]byte, KeySize)

	ct1, _ := Encrypt(plaintext, key)
	ct2, _ := Encrypt(plaintext, key)

	if bytes.Equal(ct1, ct2) {
		t.Error("Encrypt() should produce different ciphertexts due to random nonce")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	key := make([]byte, KeySize)
	ciphertext, _ := Encrypt([]byte("secret"), key)

	ciphertext[len(ciphertext)-1] ^= 0xFF

	_, err := Decrypt(ciphertext, key)
	if err == nil {
		t.Error("Decrypt() should detect tampered ciphertext")
	}
}

func TestDecrypt_InvalidKeySize(t *testing.T) {
	_, err := Decrypt([]byte("ciphertext"), []byte("short"))
	if err == nil {
		t.Error("Decrypt() should return error for invalid key size")
	}
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	_, err := Encrypt([]byte("plaintext"), []byte("short"))
	if err == nil {
		t.Error("Encrypt() should return error for invalid key size")
	}
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	key := make([]byte, KeySize)
	// Ciphertext shorter than NonceSize: Decrypt must reject before
	// touching the cipher.
	short := make([]byte, NonceSize-1)
	if _, err := Decrypt(short, key); err == nil {
		t.Fatal("Decrypt() should reject ciphertext shorter than nonce")
	}
}

func TestDecrypt_NonceWithoutCiphertext(t *testing.T) {
	// Exactly NonceSize bytes: nonce present but no payload.
	// gcm.Open should fail authentication.
	key := make([]byte, KeySize)
	noBody := make([]byte, NonceSize)
	if _, err := Decrypt(noBody, key); err == nil {
		t.Fatal("Decrypt() should reject nonce-only input (no GCM tag)")
	}
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	// Empty plaintext is valid: result is just nonce + GCM tag.
	key := make([]byte, KeySize)
	ct, err := Encrypt(nil, key)
	if err != nil {
		t.Fatalf("Encrypt(nil): %v", err)
	}
	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt of empty plaintext: %v", err)
	}
	if len(pt) != 0 {
		t.Fatalf("decrypted empty plaintext = %v, want empty", pt)
	}
}

func TestPromptPassword_NonTTYReturnsError(t *testing.T) {
	// Under `go test` os.Stdin is a pipe, not a TTY, so term.ReadPassword
	// returns an error. We just want to confirm we wrap it cleanly.
	_, err := PromptPassword("test prompt: ")
	if err == nil {
		t.Fatal("PromptPassword in non-TTY context should return an error")
	}
}

func TestPromptPasswordConfirm_NonTTYReturnsError(t *testing.T) {
	// Same reasoning: chains two PromptPassword calls; the first one
	// fails under the test runner.
	_, err := PromptPasswordConfirm()
	if err == nil {
		t.Fatal("PromptPasswordConfirm in non-TTY context should return an error")
	}
}
