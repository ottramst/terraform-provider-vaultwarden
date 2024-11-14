package keybuilder

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
)

func RSAEncrypt(data []byte, publicKey *rsa.PublicKey) (string, error) {
	encryptedBytes, err := rsa.EncryptOAEP(
		sha1.New(),
		rand.Reader,
		publicKey,
		data,
		nil)
	if err != nil {
		return "", fmt.Errorf("error encrypting data using : %w", err)
	}

	return fmt.Sprintf("%d.%s", symmetrickey.Rsa2048_OaepSha1_B64, base64.StdEncoding.EncodeToString(encryptedBytes)), nil
}

func GenerateSharedKey(publicKey *rsa.PublicKey) (string, *symmetrickey.Key, error) {
	sharedKey := make([]byte, 64)
	_, err := rand.Read(sharedKey)
	if err != nil {
		return "", nil, fmt.Errorf("error generating random bytes: %w", err)
	}

	newKey, err := symmetrickey.NewFromRawBytes(sharedKey)
	if err != nil {
		return "", nil, fmt.Errorf("error creating new symmetric crypto key")
	}

	encryptedsharedKey, err := RSAEncrypt(sharedKey, publicKey)
	if err != nil {
		return "", nil, fmt.Errorf("error encrypting shared key")
	}

	return encryptedsharedKey, newKey, nil
}
