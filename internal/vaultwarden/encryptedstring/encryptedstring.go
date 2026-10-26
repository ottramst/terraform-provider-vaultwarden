package encryptedstring

import (
	"encoding/base64"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
	"strconv"
	"strings"
)

const (
	BUFFER_ENC_TYPE_LENGTH = 1
	BUFFER_IV_LENGTH       = 16
	BUFFER_MAC_LENGTH      = 32
	BUFFER_MIN_DATA_LENGTH = 1
)

type EncryptedString struct {
	IV   []byte
	Data []byte
	Hmac []byte
	Key  symmetrickey.Key
}

func New(iv, data, hmac []byte, key symmetrickey.Key) EncryptedString {
	return EncryptedString{
		IV:   iv,
		Key:  key,
		Data: data,
		Hmac: hmac,
	}
}

func NewFromEncryptedValue(encryptedValue string) (*EncryptedString, error) {
	if len(encryptedValue) == 0 {
		return nil, fmt.Errorf("the provided encrypted value is empty")
	}

	var encPieces []string
	encString := EncryptedString{}

	headerPieces := strings.Split(encryptedValue, ".")
	if len(headerPieces) == 2 {
		s, err := strconv.ParseInt(headerPieces[0], 10, 8)
		if err != nil {
			return nil, fmt.Errorf("unable to parse encryption type from header: %w, header: %s", err, headerPieces[0])
		}
		encString.Key.EncryptionType = symmetrickey.EncryptionType(s)
		encPieces = strings.Split(headerPieces[1], "|")
	} else {
		encPieces = strings.Split(encryptedValue, "|")
		if len(encPieces) == 3 {
			encString.Key.EncryptionType = symmetrickey.AesCbc128_HmacSha256_B64
		} else {
			encString.Key.EncryptionType = symmetrickey.AesCbc256_B64
		}
	}

	switch encString.Key.EncryptionType {
	case symmetrickey.AesCbc128_HmacSha256_B64, symmetrickey.AesCbc256_HmacSha256_B64:
		if len(encPieces) != 3 {
			return nil, fmt.Errorf("bad amount of pieces (expected: 3, got: %d)", len(encPieces))
		}

		encString.IV = []byte(encPieces[0])
		encString.Data = []byte(encPieces[1])
		encString.Hmac = []byte(encPieces[2])
	case symmetrickey.AesCbc256_B64:
		if len(encPieces) != 2 {
			return nil, fmt.Errorf("bad amount of pieces (expected: 2, got: %d)", len(encPieces))
		}

		encString.IV = []byte(encPieces[0])
		encString.Data = []byte(encPieces[1])
	case symmetrickey.Rsa2048_OaepSha256_B64, symmetrickey.Rsa2048_OaepSha1_B64:
		if len(encPieces) != 1 {
			return nil, fmt.Errorf("bad amount of pieces (expected: 1, got: %d)", len(encPieces))
		}

		encString.Data = []byte(encPieces[0])
	default:
		return nil, fmt.Errorf("unsupported encryption type)")
	}

	base64DecodedIV, err := base64.StdEncoding.DecodeString(string(encString.IV))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode IV: %w", err)
	}

	base64DecodedData, err := base64.StdEncoding.DecodeString(string(encString.Data))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode data: %w", err)
	}

	base64DecodedMac, err := base64.StdEncoding.DecodeString(string(encString.Hmac))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode hmac: %w", err)
	}

	encString.IV = base64DecodedIV
	encString.Data = base64DecodedData
	encString.Hmac = base64DecodedMac
	return &encString, nil
}

func (encString *EncryptedString) String() string {
	base64EncodedIV := base64.StdEncoding.EncodeToString(encString.IV)
	base64EncodedData := base64.StdEncoding.EncodeToString(encString.Data)
	base64EncodedHmac := base64.StdEncoding.EncodeToString(encString.Hmac)

	encType := fmt.Sprintf("%d", encString.Key.EncryptionType)

	var encryptedString string
	if len(encString.IV) > 0 {
		encryptedString = encType + "." + base64EncodedIV + "|" + base64EncodedData
	} else {
		encryptedString = encType + "." + base64EncodedData
	}

	if len(encString.Hmac) > 0 {
		encryptedString = encryptedString + "|" + base64EncodedHmac
	}

	return encryptedString
}

// NewFromEncryptedBuffer parses an encrypted buffer (EncArrayBuffer), the binary form of an
// encrypted value that Bitwarden clients use for attachment files: the encryption type, the IV,
// the MAC and the data. Only AesCbc256_HmacSha256_B64 is supported.
func NewFromEncryptedBuffer(buffer []byte) (*EncryptedString, error) {
	if len(buffer) == 0 {
		return nil, fmt.Errorf("the provided encrypted buffer is empty")
	}
	encType := symmetrickey.EncryptionType(buffer[0])
	if encType != symmetrickey.AesCbc256_HmacSha256_B64 {
		return nil, fmt.Errorf("unsupported encryption type of encrypted buffer: %d", encType)
	}
	headerLength := BUFFER_ENC_TYPE_LENGTH + BUFFER_IV_LENGTH + BUFFER_MAC_LENGTH
	if len(buffer) < headerLength+BUFFER_MIN_DATA_LENGTH {
		return nil, fmt.Errorf("encrypted buffer is too short: %d bytes", len(buffer))
	}

	encString := EncryptedString{
		IV:   buffer[BUFFER_ENC_TYPE_LENGTH : BUFFER_ENC_TYPE_LENGTH+BUFFER_IV_LENGTH],
		Hmac: buffer[BUFFER_ENC_TYPE_LENGTH+BUFFER_IV_LENGTH : headerLength],
		Data: buffer[headerLength:],
	}
	encString.Key.EncryptionType = encType
	return &encString, nil
}

// Buffer returns the encrypted value as an encrypted buffer (EncArrayBuffer), see
// NewFromEncryptedBuffer.
func (encString *EncryptedString) Buffer() ([]byte, error) {
	if encString.Key.EncryptionType != symmetrickey.AesCbc256_HmacSha256_B64 {
		return nil, fmt.Errorf("unsupported encryption type of encrypted buffer: %d", encString.Key.EncryptionType)
	}
	if len(encString.IV) != BUFFER_IV_LENGTH || len(encString.Hmac) != BUFFER_MAC_LENGTH {
		return nil, fmt.Errorf("invalid IV or MAC length: %d, %d", len(encString.IV), len(encString.Hmac))
	}
	buffer := make([]byte, 0, BUFFER_ENC_TYPE_LENGTH+BUFFER_IV_LENGTH+BUFFER_MAC_LENGTH+len(encString.Data))
	buffer = append(buffer, byte(encString.Key.EncryptionType))
	buffer = append(buffer, encString.IV...)
	buffer = append(buffer, encString.Hmac...)
	buffer = append(buffer, encString.Data...)
	return buffer, nil
}
