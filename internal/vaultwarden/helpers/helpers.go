package helpers

import (
	"crypto/hmac"
	"crypto/sha256"
	"golang.org/x/crypto/hkdf"
	"hash"
)

func HMACSum(value, key []byte, algo func() hash.Hash) []byte {
	mac := hmac.New(algo, key)
	_, err := mac.Write(value)
	if err != nil {
		panic("BUG: hmac.Write should never return an error")
	}
	return mac.Sum(nil)
}

func HKDFExpand(value, key []byte, algo func() hash.Hash, length int) []byte {
	encKey := hkdf.Expand(sha256.New, value, key)
	newEncKey := make([]byte, length)
	_, err := encKey.Read(newEncKey)
	if err != nil {
		panic("BUG: hkdf.Expand should never return an error")
	}
	return newEncKey
}
