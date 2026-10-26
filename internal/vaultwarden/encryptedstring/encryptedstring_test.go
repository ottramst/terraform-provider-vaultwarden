package encryptedstring_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/encryptedstring"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
)

func TestEncryptedBufferRoundTrip(t *testing.T) {
	raw := make([]byte, 64)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	key, err := symmetrickey.NewFromRawBytes(raw)
	if err != nil {
		t.Fatal(err)
	}

	plain := []byte("attachment content")
	encrypted, err := crypt.Encrypt(plain, *key)
	if err != nil {
		t.Fatal(err)
	}
	buffer, err := encrypted.Buffer()
	if err != nil {
		t.Fatal(err)
	}
	if buffer[0] != byte(symmetrickey.AesCbc256_HmacSha256_B64) || len(buffer) != 1+16+32+len(encrypted.Data) {
		t.Fatalf("unexpected buffer layout: type %d, length %d", buffer[0], len(buffer))
	}

	parsed, err := encryptedstring.NewFromEncryptedBuffer(buffer)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := crypt.Decrypt(parsed, key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("decrypted %q, want %q", decrypted, plain)
	}

	// A modified buffer fails the MAC check
	buffer[len(buffer)-1] ^= 1
	parsed, err = encryptedstring.NewFromEncryptedBuffer(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := crypt.Decrypt(parsed, key); err == nil {
		t.Fatal("a modified buffer was decrypted")
	}
}

func TestEncryptedBufferInvalid(t *testing.T) {
	for name, buffer := range map[string][]byte{
		"empty":        nil,
		"wrong type":   append([]byte{0}, make([]byte, 64)...),
		"no data":      append([]byte{2}, make([]byte, 48)...),
		"short header": {2, 1, 2, 3},
	} {
		if _, err := encryptedstring.NewFromEncryptedBuffer(buffer); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
