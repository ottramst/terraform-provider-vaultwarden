package vaultwarden

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/encryptedstring"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
)

// cipherAttachment is an attachment of a cipher as returned by Vaultwarden. The file name is
// encrypted with the item key, and the file with the attachment key, which is encrypted with the
// item key. Attachments uploaded by very old clients have no key: their file is encrypted with
// the item key.
type cipherAttachment struct {
	ID       string      `json:"id"`
	URL      string      `json:"url"`
	FileName *string     `json:"fileName"`
	Key      *string     `json:"key"`
	Size     json.Number `json:"size"`
	SizeName string      `json:"sizeName"`
}

// attachmentFileUploadTypeDirect is the upload type of an attachment uploaded to Vaultwarden
// itself, the only one Vaultwarden uses.
const attachmentFileUploadTypeDirect = 0

// attachmentError maps the errors of the attachment endpoints. Vaultwarden answers 400 for an
// attachment that does not exist or belongs to another cipher.
func attachmentError(itemID, attachmentID string, err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest &&
		(strings.Contains(apiErr.Body, "Attachment doesn't exist") || strings.Contains(apiErr.Body, "Attachment from other cipher")) {
		return fmt.Errorf("attachment %s of item %s: %w", attachmentID, itemID, ErrNotFound)
	}
	return cipherError(itemID, err)
}

// cipherKey returns the key that encrypts the fields and attachment keys of a cipher.
func (c *Client) cipherKey(ctx context.Context, itemID string) (symmetrickey.Key, error) {
	current, _, err := c.getCipher(ctx, itemID)
	if err != nil {
		return symmetrickey.Key{}, err
	}
	ownerKey, err := c.ownerKey(ctx, valueOf(current.OrganizationID))
	if err != nil {
		return symmetrickey.Key{}, err
	}
	key, err := itemKey(current.Key, ownerKey)
	if err != nil {
		return symmetrickey.Key{}, fmt.Errorf("item %s: %w", itemID, err)
	}
	return key, nil
}

// GetAttachment returns an attachment of an item, with its file name decrypted.
func (c *Client) GetAttachment(ctx context.Context, itemID, attachmentID string) (*models.ItemAttachment, error) {
	item, err := c.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	for _, attachment := range item.Attachments {
		if attachment.ID == attachmentID {
			return &attachment, nil
		}
	}
	return nil, fmt.Errorf("attachment %s of item %s: %w", attachmentID, itemID, ErrNotFound)
}

// CreateAttachment attaches a file to an item, like Bitwarden clients do: the file is encrypted
// with a new attachment key and uploaded as an encrypted buffer, and its name and the attachment
// key are encrypted with the item key. Empty files are not supported.
func (c *Client) CreateAttachment(ctx context.Context, itemID, fileName string, content []byte) (*models.ItemAttachment, error) {
	if len(content) == 0 {
		return nil, errors.New("the attachment is empty: empty files are not supported")
	}
	if fileName == "" {
		return nil, errors.New("the file name of the attachment is empty")
	}

	key, err := c.cipherKey(ctx, itemID)
	if err != nil {
		return nil, err
	}
	attachmentKey, encryptedKey, err := newObjectKey(key)
	if err != nil {
		return nil, err
	}
	encryptedFile, err := crypt.Encrypt(content, attachmentKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt the attachment: %w", err)
	}
	buffer, err := encryptedFile.Buffer()
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt the attachment: %w", err)
	}
	encryptedFileName, err := encryptString(fileName, key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt the file name of the attachment: %w", err)
	}

	var created struct {
		AttachmentID   string `json:"attachmentId"`
		FileUploadType int    `json:"fileUploadType"`
	}
	if _, err := c.doRequest(ctx, http.MethodPost, "/api/ciphers/"+itemID+"/attachment/v2", map[string]any{
		"key":      encryptedKey,
		"fileName": encryptedFileName,
		"fileSize": len(buffer),
	}, &created); err != nil {
		return nil, fmt.Errorf("failed to create attachment: %w", cipherError(itemID, err))
	}

	// Vaultwarden creates the attachment before its file is uploaded: delete it if the upload fails
	uploadErr := fmt.Errorf("unsupported upload type %d", created.FileUploadType)
	if created.FileUploadType == attachmentFileUploadTypeDirect {
		_, uploadErr = c.doRequest(ctx, http.MethodPost, "/api/ciphers/"+itemID+"/attachment/"+created.AttachmentID, multipartFile{
			field:    "data",
			fileName: encryptedFileName,
			data:     buffer,
		}, nil)
	}
	if uploadErr != nil {
		if err := c.DeleteAttachment(ctx, itemID, created.AttachmentID); err != nil {
			return nil, fmt.Errorf("failed to upload attachment: %w (deleting the incomplete attachment %s also failed: %v)", uploadErr, created.AttachmentID, err)
		}
		return nil, fmt.Errorf("failed to upload attachment: %w", uploadErr)
	}

	return c.GetAttachment(ctx, itemID, created.AttachmentID)
}

// GetAttachmentContent downloads an attachment of an item and returns its decrypted content.
func (c *Client) GetAttachmentContent(ctx context.Context, itemID, attachmentID string) ([]byte, error) {
	key, err := c.cipherKey(ctx, itemID)
	if err != nil {
		return nil, err
	}

	var attachment cipherAttachment
	if _, err := c.doRequest(ctx, http.MethodGet, "/api/ciphers/"+itemID+"/attachment/"+attachmentID, nil, &attachment); err != nil {
		return nil, fmt.Errorf("failed to get attachment: %w", attachmentError(itemID, attachmentID, err))
	}

	// The download URL is authorized by its token
	var buffer []byte
	if _, err := c.doUnauthenticatedRequest(ctx, http.MethodGet, attachmentDownloadPath(attachment.URL, itemID), nil, &buffer); err != nil {
		return nil, fmt.Errorf("failed to download attachment %s of item %s: %w", attachmentID, itemID, err)
	}

	fileKey := key
	if valueOf(attachment.Key) != "" {
		raw, err := decryptString(*attachment.Key, key)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt the key of attachment %s: %w", attachmentID, err)
		}
		decryptedKey, err := symmetrickey.NewFromRawBytes([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid key of attachment %s: %w", attachmentID, err)
		}
		fileKey = *decryptedKey
	}

	encrypted, err := encryptedstring.NewFromEncryptedBuffer(buffer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse attachment %s: %w", attachmentID, err)
	}
	content, err := crypt.Decrypt(encrypted, &fileKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt attachment %s: %w", attachmentID, err)
	}
	return content, nil
}

// attachmentDownloadPath returns where to download an attachment from. Vaultwarden builds the
// download URL of a file it stores itself from its DOMAIN setting, which may not be the endpoint
// the provider uses, so such a URL is taken relative to the endpoint. Other URLs, such as those
// of object storage, are used as they are.
func attachmentDownloadPath(downloadURL, itemID string) string {
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return downloadURL
	}
	i := strings.Index(parsed.Path, "/attachments/"+itemID+"/")
	if i < 0 {
		return downloadURL
	}
	path := parsed.Path[i:]
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	return path
}

// DeleteAttachment deletes an attachment of an item. Deleting an attachment that does not exist
// succeeds.
func (c *Client) DeleteAttachment(ctx context.Context, itemID, attachmentID string) error {
	if _, err := c.doRequest(ctx, http.MethodDelete, "/api/ciphers/"+itemID+"/attachment/"+attachmentID, nil, nil); err != nil {
		if err = attachmentError(itemID, attachmentID, err); IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete attachment: %w", err)
	}
	return nil
}
