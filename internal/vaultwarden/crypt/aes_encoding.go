package crypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

func pkcs5Unpadding(src []byte, blockSize int) ([]byte, error) {
	srcLen := len(src)
	paddingLen := int(src[srcLen-1])
	if paddingLen == srcLen {
		return []byte{}, nil
	}

	if paddingLen > blockSize {
		return nil, fmt.Errorf("bad padding size")
	}
	return src[:srcLen-paddingLen], nil
}

func aes256Decode(cipherText []byte, key []byte, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("error creating new cipher block: %w", err)
	}

	plainText := make([]byte, len(cipherText))

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plainText, cipherText)

	return pkcs5Unpadding(plainText, block.BlockSize())
}

func pkcs5Padding(cipherText []byte, blockSize int) []byte {
	padding := blockSize - len(cipherText)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(cipherText, padtext...)
}

func aes256Encode(plainText []byte, key []byte, iv []byte, blockSize int) ([]byte, error) {
	plainTextPadded := pkcs5Padding(plainText, blockSize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("error creating new cipher block: %w", err)
	}

	cipherText := make([]byte, len(plainTextPadded))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherText, plainTextPadded)

	return cipherText, nil
}
