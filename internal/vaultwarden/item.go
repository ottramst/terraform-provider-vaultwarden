package vaultwarden

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
)

// maxPasswordHistory is the number of previous passwords Bitwarden clients keep for a login.
const maxPasswordHistory = 5

// cipher is a vault item as returned by Vaultwarden, with its fields encrypted. Fields that the
// provider does not manage, such as the password history and the passkeys of a login, are kept
// as they are on updates.
type cipher struct {
	ID              string             `json:"id"`
	Type            models.ItemType    `json:"type"`
	OrganizationID  *string            `json:"organizationId"`
	FolderID        *string            `json:"folderId"`
	CollectionIDs   []string           `json:"collectionIds"`
	Key             *string            `json:"key"`
	Name            string             `json:"name"`
	Notes           *string            `json:"notes"`
	Favorite        bool               `json:"favorite"`
	Reprompt        int                `json:"reprompt"`
	Fields          []cipherField      `json:"fields"`
	Login           json.RawMessage    `json:"login"`
	PasswordHistory json.RawMessage    `json:"passwordHistory"`
	Attachments     []cipherAttachment `json:"attachments"`
	ArchivedDate    *string            `json:"archivedDate"`
	DeletedDate     *string            `json:"deletedDate"`
	RevisionDate    string             `json:"revisionDate"`
}

// cipherField is a custom field of a cipher.
type cipherField struct {
	Name     *string          `json:"name"`
	Value    *string          `json:"value"`
	Type     models.FieldType `json:"type"`
	LinkedID *int             `json:"linkedId"`
}

// cipherLoginURI is a URI of a login cipher. Bitwarden clients ignore the URIs of a cipher with
// an item key unless uriChecksum holds the encrypted base64 SHA-256 hash of the URI.
type cipherLoginURI struct {
	URI         *string              `json:"uri"`
	Match       *models.URIMatchType `json:"match"`
	URIChecksum *string              `json:"uriChecksum"`
}

// cipherPasswordHistory is an entry of the password history of a login cipher.
type cipherPasswordHistory struct {
	Password     string `json:"password"`
	LastUsedDate string `json:"lastUsedDate"`
}

// cipherError maps the errors of the cipher endpoints. Vaultwarden answers 400 for a cipher that
// does not exist or that the user cannot access, and 404 for an ID that is not a UUID.
func cipherError(itemID string, err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound ||
		(apiErr.StatusCode == http.StatusBadRequest && (strings.Contains(apiErr.Body, "Cipher doesn't exist") ||
			strings.Contains(apiErr.Body, "Cipher does not exist") ||
			strings.Contains(apiErr.Body, "Cipher is not owned by user")))) {
		return fmt.Errorf("item %s: %w", itemID, ErrNotFound)
	}
	return err
}

// ownerKey returns the key that encrypts the items of an organization, or of the provider
// user's personal vault when orgID is empty.
func (c *Client) ownerKey(ctx context.Context, orgID string) (symmetrickey.Key, error) {
	if orgID == "" {
		return c.UserKey(ctx)
	}
	if err := c.ensureUserAuth(ctx); err != nil {
		return symmetrickey.Key{}, err
	}
	secret, ok := c.OrganizationSecret(orgID)
	if !ok {
		// The organization may have been created by another client since the login
		if err := c.refreshOrganizationSecrets(ctx); err != nil {
			return symmetrickey.Key{}, err
		}
		secret, ok = c.OrganizationSecret(orgID)
	}
	if !ok {
		return symmetrickey.Key{}, fmt.Errorf("the key of organization %s is not available: the provider user is not a confirmed member of it", orgID)
	}
	return secret.Key, nil
}

// itemKey returns the key that encrypts the fields of a cipher: its item key, decrypted with the
// owner key, or the owner key itself for ciphers without an item key.
func itemKey(encryptedKey *string, ownerKey symmetrickey.Key) (symmetrickey.Key, error) {
	if encryptedKey == nil || *encryptedKey == "" {
		return ownerKey, nil
	}
	raw, err := decryptString(*encryptedKey, ownerKey)
	if err != nil {
		return symmetrickey.Key{}, fmt.Errorf("failed to decrypt the item key: %w", err)
	}
	key, err := symmetrickey.NewFromRawBytes([]byte(raw))
	if err != nil {
		return symmetrickey.Key{}, fmt.Errorf("invalid item key: %w", err)
	}
	return *key, nil
}

// newObjectKey generates the key of an item or an attachment, and returns it with its
// encryption with parentKey: the owner key for an item, the item key for an attachment.
func newObjectKey(parentKey symmetrickey.Key) (symmetrickey.Key, string, error) {
	raw := make([]byte, 64)
	if _, err := rand.Read(raw); err != nil {
		return symmetrickey.Key{}, "", fmt.Errorf("failed to generate a key: %w", err)
	}
	key, err := symmetrickey.NewFromRawBytes(raw)
	if err != nil {
		return symmetrickey.Key{}, "", err
	}
	encrypted, err := crypt.EncryptAsString(raw, parentKey)
	if err != nil {
		return symmetrickey.Key{}, "", fmt.Errorf("failed to encrypt the key: %w", err)
	}
	return *key, encrypted, nil
}

// uriChecksum returns the checksum of a login URI as Bitwarden clients compute it.
func uriChecksum(uri string) string {
	sum := sha256.Sum256([]byte(uri))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// nullIfEmpty returns nil for an empty string, which Vaultwarden stores as null.
func nullIfEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// valueOf returns the value of a string pointer, or an empty string for nil.
func valueOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// GetItem returns an item with its fields decrypted. Items in the trash are reported as not found.
func (c *Client) GetItem(ctx context.Context, itemID string) (*models.Item, error) {
	encrypted, _, err := c.getCipher(ctx, itemID)
	if err != nil {
		return nil, err
	}
	return c.decryptCipher(ctx, encrypted)
}

// getCipher returns a cipher and its raw JSON.
func (c *Client) getCipher(ctx context.Context, itemID string) (*cipher, map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if _, err := c.doRequest(ctx, http.MethodGet, "/api/ciphers/"+itemID, nil, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to get item: %w", cipherError(itemID, err))
	}

	encrypted, err := parseCipher(raw)
	if err != nil {
		return nil, nil, err
	}
	if encrypted.DeletedDate != nil {
		return nil, nil, fmt.Errorf("item %s is in the trash: %w", itemID, ErrNotFound)
	}
	return encrypted, raw, nil
}

// parseCipher decodes the raw JSON of a cipher.
func parseCipher(raw map[string]json.RawMessage) (*cipher, error) {
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var encrypted cipher
	if err := json.Unmarshal(body, &encrypted); err != nil {
		return nil, fmt.Errorf("failed to decode item: %w", err)
	}
	return &encrypted, nil
}

// decryptCipher decrypts a cipher with the key of its owner.
func (c *Client) decryptCipher(ctx context.Context, encrypted *cipher) (*models.Item, error) {
	ownerKey, err := c.ownerKey(ctx, valueOf(encrypted.OrganizationID))
	if err != nil {
		return nil, err
	}
	key, err := itemKey(encrypted.Key, ownerKey)
	if err != nil {
		return nil, fmt.Errorf("item %s: %w", encrypted.ID, err)
	}

	item := &models.Item{
		ID:             encrypted.ID,
		Type:           encrypted.Type,
		OrganizationID: valueOf(encrypted.OrganizationID),
		CollectionIDs:  encrypted.CollectionIDs,
		FolderID:       valueOf(encrypted.FolderID),
		Favorite:       encrypted.Favorite,
		Reprompt:       encrypted.Reprompt == 1,
		RevisionDate:   encrypted.RevisionDate,
		HasKey:         encrypted.Key != nil && *encrypted.Key != "",
	}

	decrypt := func(what string, value *string) (string, error) {
		decrypted, err := decryptString(valueOf(value), key)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt the %s of item %s: %w", what, encrypted.ID, err)
		}
		return decrypted, nil
	}

	if item.Name, err = decrypt("name", &encrypted.Name); err != nil {
		return nil, err
	}
	if item.Notes, err = decrypt("notes", encrypted.Notes); err != nil {
		return nil, err
	}

	for _, field := range encrypted.Fields {
		decrypted := models.ItemField{Type: field.Type, LinkedID: field.LinkedID}
		if decrypted.Name, err = decrypt("field name", field.Name); err != nil {
			return nil, err
		}
		if decrypted.Value, err = decrypt("field value", field.Value); err != nil {
			return nil, err
		}
		item.Fields = append(item.Fields, decrypted)
	}

	if len(encrypted.PasswordHistory) > 0 && string(encrypted.PasswordHistory) != "null" {
		var history []cipherPasswordHistory
		if err := json.Unmarshal(encrypted.PasswordHistory, &history); err != nil {
			return nil, fmt.Errorf("failed to decode the password history of item %s: %w", encrypted.ID, err)
		}
		for _, entry := range history {
			password, err := decrypt("password history", &entry.Password)
			if err != nil {
				return nil, err
			}
			item.PasswordHistory = append(item.PasswordHistory, models.ItemPasswordHistory{Password: password, LastUsedDate: entry.LastUsedDate})
		}
	}

	if encrypted.Type == models.ItemTypeLogin && len(encrypted.Login) > 0 && string(encrypted.Login) != "null" {
		var login struct {
			Username *string          `json:"username"`
			Password *string          `json:"password"`
			TOTP     *string          `json:"totp"`
			URIs     []cipherLoginURI `json:"uris"`
		}
		if err := json.Unmarshal(encrypted.Login, &login); err != nil {
			return nil, fmt.Errorf("failed to decode the login of item %s: %w", encrypted.ID, err)
		}

		item.Login = &models.ItemLogin{}
		if item.Login.Username, err = decrypt("username", login.Username); err != nil {
			return nil, err
		}
		if item.Login.Password, err = decrypt("password", login.Password); err != nil {
			return nil, err
		}
		if item.Login.TOTP, err = decrypt("TOTP", login.TOTP); err != nil {
			return nil, err
		}
		for _, uri := range login.URIs {
			if uri.URI == nil {
				continue
			}
			decrypted, err := decrypt("URI", uri.URI)
			if err != nil {
				return nil, err
			}
			// Like Bitwarden clients, ignore the URIs of an item with an item key whose
			// checksum is missing or wrong
			if item.HasKey {
				checksum, err := decryptString(valueOf(uri.URIChecksum), key)
				if err != nil || checksum != uriChecksum(decrypted) {
					continue
				}
			}
			item.Login.URIs = append(item.Login.URIs, models.ItemLoginURI{URI: decrypted, Match: uri.Match})
		}
	}

	for _, attachment := range encrypted.Attachments {
		fileName, err := decrypt("attachment file name", attachment.FileName)
		if err != nil {
			return nil, err
		}
		size, err := attachment.Size.Int64()
		if err != nil {
			return nil, fmt.Errorf("invalid size of attachment %s of item %s: %w", attachment.ID, encrypted.ID, err)
		}
		item.Attachments = append(item.Attachments, models.ItemAttachment{
			ID:       attachment.ID,
			FileName: fileName,
			Size:     size,
			SizeName: attachment.SizeName,
		})
	}

	return item, nil
}

// encryptCipher returns the request body for an item, encrypted with key. current is the raw
// JSON of the item on the server for updates, whose unmanaged fields are kept.
func encryptCipher(item models.Item, key symmetrickey.Key, current map[string]json.RawMessage, currentPassword string) (map[string]any, error) {
	encrypt := func(what, value string) (*string, error) {
		encrypted, err := encryptString(value, key)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt the %s: %w", what, err)
		}
		return nullIfEmpty(encrypted), nil
	}

	body := map[string]any{
		"type":     item.Type,
		"favorite": item.Favorite,
		"reprompt": 0,
		"folderId": nullIfEmpty(item.FolderID),
	}
	if item.Reprompt {
		body["reprompt"] = 1
	}
	if item.OrganizationID != "" {
		body["organizationId"] = item.OrganizationID
	}

	name, err := encrypt("name", item.Name)
	if err != nil {
		return nil, err
	}
	body["name"] = name
	if body["notes"], err = encrypt("notes", item.Notes); err != nil {
		return nil, err
	}

	fields := []cipherField{}
	for _, field := range item.Fields {
		encrypted := cipherField{Type: field.Type, LinkedID: field.LinkedID}
		if encrypted.Name, err = encrypt("field name", field.Name); err != nil {
			return nil, err
		}
		if encrypted.Value, err = encrypt("field value", field.Value); err != nil {
			return nil, err
		}
		fields = append(fields, encrypted)
	}
	body["fields"] = fields

	// Keep fields the provider does not manage
	for _, preserved := range []string{"passwordHistory", "archivedDate"} {
		if value, ok := current[preserved]; ok {
			body[preserved] = value
		}
	}

	switch item.Type {
	case models.ItemTypeSecureNote:
		body["secureNote"] = map[string]int{"type": 0}
	case models.ItemTypeLogin:
		login := map[string]any{}
		// Keep the login fields the provider does not manage, such as passkeys
		if value, ok := current["login"]; ok && string(value) != "null" {
			var currentLogin map[string]json.RawMessage
			if err := json.Unmarshal(value, &currentLogin); err != nil {
				return nil, fmt.Errorf("failed to decode the current login: %w", err)
			}
			for k, v := range currentLogin {
				login[k] = v
			}
			// Vaultwarden adds the first URI as "uri" for older clients
			delete(login, "uri")
		}

		data := item.Login
		if data == nil {
			data = &models.ItemLogin{}
		}
		if login["username"], err = encrypt("username", data.Username); err != nil {
			return nil, err
		}
		if login["password"], err = encrypt("password", data.Password); err != nil {
			return nil, err
		}
		if login["totp"], err = encrypt("TOTP", data.TOTP); err != nil {
			return nil, err
		}

		uris := []cipherLoginURI{}
		for _, uri := range data.URIs {
			encrypted := cipherLoginURI{Match: uri.Match}
			if encrypted.URI, err = encrypt("URI", uri.URI); err != nil {
				return nil, err
			}
			if encrypted.URIChecksum, err = encrypt("URI checksum", uriChecksum(uri.URI)); err != nil {
				return nil, err
			}
			uris = append(uris, encrypted)
		}
		login["uris"] = uris

		// Like Bitwarden clients, keep the previous password in the password history
		if current != nil && currentPassword != data.Password {
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			login["passwordRevisionDate"] = now
			if currentPassword != "" {
				var history []json.RawMessage
				if value, ok := current["passwordHistory"]; ok && string(value) != "null" {
					if err := json.Unmarshal(value, &history); err != nil {
						return nil, fmt.Errorf("failed to decode the password history: %w", err)
					}
				}
				previous, err := encrypt("password history", currentPassword)
				if err != nil {
					return nil, err
				}
				entry, err := json.Marshal(cipherPasswordHistory{Password: *previous, LastUsedDate: now})
				if err != nil {
					return nil, err
				}
				history = append([]json.RawMessage{entry}, history...)
				if len(history) > maxPasswordHistory {
					history = history[:maxPasswordHistory]
				}
				body["passwordHistory"] = history
			}
		}
		body["login"] = login
	default:
		return nil, fmt.Errorf("unsupported item type %d", item.Type)
	}

	return body, nil
}

// providerUserID returns the ID of the provider user, which Vaultwarden 1.37 and later require
// as encryptedFor of every item written.
func (c *Client) providerUserID(ctx context.Context) (string, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	if err := c.ensureUserSessionLocked(ctx); err != nil {
		return "", err
	}
	return c.AuthState.UserID, nil
}

// CreateItem creates an item in the personal vault of the provider user, or in an organization
// when item.OrganizationID is set. The item's fields are encrypted with a new item key, like
// current Bitwarden clients do.
func (c *Client) CreateItem(ctx context.Context, item models.Item) (*models.Item, error) {
	userID, err := c.providerUserID(ctx)
	if err != nil {
		return nil, err
	}
	ownerKey, err := c.ownerKey(ctx, item.OrganizationID)
	if err != nil {
		return nil, err
	}
	key, encryptedKey, err := newObjectKey(ownerKey)
	if err != nil {
		return nil, err
	}

	body, err := encryptCipher(item, key, nil, "")
	if err != nil {
		return nil, err
	}
	body["key"] = encryptedKey
	body["encryptedFor"] = userID

	var raw map[string]json.RawMessage
	if item.OrganizationID == "" {
		_, err = c.doRequest(ctx, http.MethodPost, "/api/ciphers", body, &raw)
	} else {
		collectionIDs := item.CollectionIDs
		if collectionIDs == nil {
			collectionIDs = []string{}
		}
		_, err = c.doRequest(ctx, http.MethodPost, "/api/ciphers/create", map[string]any{
			"cipher":        body,
			"collectionIds": collectionIDs,
		}, &raw)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create item: %w", err)
	}

	created, err := parseCipher(raw)
	if err != nil {
		return nil, err
	}
	return c.decryptCipher(ctx, created)
}

// UpdateItem updates an item. Its owner cannot change. Fields the provider does not manage, such
// as the password history, passkeys and attachments, are kept, and the item keeps its item key
// so that they stay readable. A changed password is added to the password history.
func (c *Client) UpdateItem(ctx context.Context, item models.Item) (*models.Item, error) {
	current, raw, err := c.getCipher(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	if valueOf(current.OrganizationID) != item.OrganizationID {
		return nil, fmt.Errorf("item %s belongs to another owner: moving items between the personal vault and organizations is not supported", item.ID)
	}
	if current.Type != item.Type {
		return nil, fmt.Errorf("item %s has type %d, not %d", item.ID, current.Type, item.Type)
	}

	userID, err := c.providerUserID(ctx)
	if err != nil {
		return nil, err
	}
	ownerKey, err := c.ownerKey(ctx, item.OrganizationID)
	if err != nil {
		return nil, err
	}
	key, err := itemKey(current.Key, ownerKey)
	if err != nil {
		return nil, fmt.Errorf("item %s: %w", item.ID, err)
	}

	currentPassword := ""
	if item.Type == models.ItemTypeLogin {
		decrypted, err := c.decryptCipher(ctx, current)
		if err != nil {
			return nil, err
		}
		if decrypted.Login != nil {
			currentPassword = decrypted.Login.Password
		}
	}

	body, err := encryptCipher(item, key, raw, currentPassword)
	if err != nil {
		return nil, err
	}
	body["key"] = current.Key
	body["encryptedFor"] = userID

	if _, err := c.doRequest(ctx, http.MethodPut, "/api/ciphers/"+item.ID, body, nil); err != nil {
		return nil, fmt.Errorf("failed to update item: %w", cipherError(item.ID, err))
	}

	if item.OrganizationID != "" && !sameElements(item.CollectionIDs, current.CollectionIDs) {
		collectionIDs := item.CollectionIDs
		if collectionIDs == nil {
			collectionIDs = []string{}
		}
		if _, err := c.doRequest(ctx, http.MethodPut, "/api/ciphers/"+item.ID+"/collections_v2",
			map[string][]string{"collectionIds": collectionIDs}, nil); err != nil {
			return nil, fmt.Errorf("failed to update the collections of item %s: %w", item.ID, err)
		}
	}

	return c.GetItem(ctx, item.ID)
}

// sameElements reports whether two lists hold the same strings, in any order.
func sameElements(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

// DeleteItem permanently deletes an item, without moving it to the trash. Deleting an item that
// does not exist succeeds.
func (c *Client) DeleteItem(ctx context.Context, itemID string) error {
	if _, err := c.doRequest(ctx, http.MethodDelete, "/api/ciphers/"+itemID, nil, nil); err != nil {
		if err = cipherError(itemID, err); IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete item: %w", err)
	}
	return nil
}
