package crypt

import (
	"crypto/sha256"
	"encoding/base64"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
	"golang.org/x/crypto/pbkdf2"
)

func HashPassword(password string, key symmetrickey.Key, localAuthorization bool) string {
	iterations := 1
	if localAuthorization {
		iterations = 2
	}
	derivedKey := pbkdf2.Key(key.Key, []byte(password), iterations, 32, sha256.New)
	return base64.StdEncoding.EncodeToString(derivedKey)
}
