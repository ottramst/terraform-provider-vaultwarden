package vaultwarden_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"testing"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/encryptedstring"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

// These tests construct and inspect ciphers the way Bitwarden clients do, with keys derived
// independently of the client's login, to check that items round-trip between the provider
// and Bitwarden clients.

// itemTestSetup returns the test client, the provider user's ID and the user key, decrypted
// from the profile with the master password.
func itemTestSetup(t *testing.T) (context.Context, *vaultwarden.Client, string, symmetrickey.Key) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test, set TF_ACC=1 to run it against a Vaultwarden server")
	}

	ctx := context.Background()
	client, err := test.GetTestClient(ctx, t)
	must(t, err)

	prelogin, err := client.PreLogin(ctx)
	must(t, err)
	masterKey, err := keybuilder.BuildPreloginKey(test.TestPassword, test.TestEmail, &models.KdfConfiguration{
		KdfType:        prelogin.Kdf,
		KdfIterations:  prelogin.KdfIterations,
		KdfMemory:      prelogin.KdfMemory,
		KdfParallelism: prelogin.KdfParallelism,
	})
	must(t, err)
	profile, err := client.GetProfile(ctx)
	must(t, err)
	userKey, err := crypt.DecryptEncryptionKey(profile.Key, *masterKey)
	must(t, err)

	userID, err := client.ProviderUserID(ctx)
	must(t, err)
	if userID != profile.ID {
		t.Fatalf("provider user ID %q, profile ID %q", userID, profile.ID)
	}

	return ctx, client, userID, *userKey
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func encrypt(t *testing.T, value string, key symmetrickey.Key) string {
	t.Helper()
	encrypted, err := crypt.EncryptAsString([]byte(value), key)
	must(t, err)
	return encrypted
}

func decrypt(t *testing.T, value string, key symmetrickey.Key) string {
	t.Helper()
	encrypted, err := encryptedstring.NewFromEncryptedValue(value)
	must(t, err)
	decrypted, err := crypt.Decrypt(encrypted, &key)
	must(t, err)
	return string(decrypted)
}

func checksum(uri string) string {
	sum := sha256.Sum256([]byte(uri))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// newKey returns a new item key and its encryption with key.
func newKey(t *testing.T, key symmetrickey.Key) (symmetrickey.Key, string) {
	t.Helper()
	raw := make([]byte, 64)
	_, err := rand.Read(raw)
	must(t, err)
	itemKey, err := symmetrickey.NewFromRawBytes(raw)
	must(t, err)
	encrypted, err := crypt.EncryptAsString(raw, key)
	must(t, err)
	return *itemKey, encrypted
}

// rawCipher is the part of a cipher as returned by Vaultwarden that the tests inspect.
type rawCipher struct {
	Key   *string `json:"key"`
	Name  string  `json:"name"`
	Login struct {
		Password           *string `json:"password"`
		AutofillOnPageLoad *bool   `json:"autofillOnPageLoad"`
		URIs               []struct {
			URI         string  `json:"uri"`
			URIChecksum *string `json:"uriChecksum"`
		} `json:"uris"`
	} `json:"login"`
	PasswordHistory []struct {
		Password string `json:"password"`
	} `json:"passwordHistory"`
}

func getRawCipher(ctx context.Context, t *testing.T, client *vaultwarden.Client, id string) rawCipher {
	t.Helper()
	var raw rawCipher
	must(t, client.DoRequest(ctx, http.MethodGet, "/api/ciphers/"+id, nil, &raw))
	return raw
}

func postCipher(ctx context.Context, t *testing.T, client *vaultwarden.Client, body map[string]any) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	must(t, client.DoRequest(ctx, http.MethodPost, "/api/ciphers", body, &created))
	t.Cleanup(func() { _ = client.DeleteItem(context.Background(), created.ID) })
	return created.ID
}

// A cipher without an item key, as older Bitwarden clients create them: the fields are
// encrypted with the user key, and URIs have no checksum.
func TestAccItemWithoutItemKey(t *testing.T) {
	ctx, client, userID, userKey := itemTestSetup(t)

	id := postCipher(ctx, t, client, map[string]any{
		"type":         1,
		"encryptedFor": userID,
		"name":         encrypt(t, "Keyless", userKey),
		"notes":        encrypt(t, "Some notes", userKey),
		"fields": []map[string]any{
			{"name": encrypt(t, "API key", userKey), "value": encrypt(t, "secret", userKey), "type": 1},
		},
		"login": map[string]any{
			"username": encrypt(t, "user", userKey),
			"password": encrypt(t, "old password", userKey),
			"uris":     []map[string]any{{"uri": encrypt(t, "https://example.com", userKey), "match": 1}},
		},
	})

	item, err := client.GetItem(ctx, id)
	must(t, err)
	if item.HasKey || item.Name != "Keyless" || item.Notes != "Some notes" || item.Login.Username != "user" ||
		item.Login.Password != "old password" || len(item.Fields) != 1 || item.Fields[0].Value != "secret" ||
		item.Fields[0].Type != models.FieldTypeHidden || len(item.Login.URIs) != 1 ||
		item.Login.URIs[0].URI != "https://example.com" || *item.Login.URIs[0].Match != models.URIMatchHost {
		t.Fatalf("unexpected item: %+v, login %+v", item, item.Login)
	}

	// An update keeps the item without an item key, and adds the old password to the history
	item.Login.Password = "new password"
	updated, err := client.UpdateItem(ctx, *item)
	must(t, err)
	if updated.HasKey || updated.Login.Password != "new password" || len(updated.PasswordHistory) != 1 ||
		updated.PasswordHistory[0].Password != "old password" {
		t.Fatalf("unexpected item after update: %+v, login %+v", updated, updated.Login)
	}

	raw := getRawCipher(ctx, t, client, id)
	if raw.Key != nil {
		t.Fatalf("the update added an item key")
	}
	if got := decrypt(t, raw.Name, userKey); got != "Keyless" {
		t.Fatalf("name decrypted with the user key: %q", got)
	}
	if got := decrypt(t, raw.PasswordHistory[0].Password, userKey); got != "old password" {
		t.Fatalf("password history decrypted with the user key: %q", got)
	}
}

// A cipher with an item key, as current Bitwarden clients create them. URIs without a valid
// checksum are ignored, and fields the provider does not manage are kept on updates.
func TestAccItemWithItemKey(t *testing.T) {
	ctx, client, userID, userKey := itemTestSetup(t)
	itemKey, encryptedItemKey := newKey(t, userKey)

	id := postCipher(ctx, t, client, map[string]any{
		"type":         1,
		"encryptedFor": userID,
		"key":          encryptedItemKey,
		"name":         encrypt(t, "Keyed", itemKey),
		"passwordHistory": []map[string]any{
			{"password": encrypt(t, "older password", itemKey), "lastUsedDate": "2025-01-01T00:00:00.000Z"},
		},
		"login": map[string]any{
			"username":           encrypt(t, "user", itemKey),
			"password":           encrypt(t, "password", itemKey),
			"autofillOnPageLoad": true,
			"uris": []map[string]any{
				{"uri": encrypt(t, "https://valid.example.com", itemKey), "uriChecksum": encrypt(t, checksum("https://valid.example.com"), itemKey)},
				{"uri": encrypt(t, "https://missing.example.com", itemKey)},
				{"uri": encrypt(t, "https://wrong.example.com", itemKey), "uriChecksum": encrypt(t, checksum("https://other.example.com"), itemKey)},
			},
		},
	})

	item, err := client.GetItem(ctx, id)
	must(t, err)
	if !item.HasKey || item.Name != "Keyed" || item.Login.Username != "user" || item.Login.Password != "password" ||
		len(item.PasswordHistory) != 1 || item.PasswordHistory[0].Password != "older password" {
		t.Fatalf("unexpected item: %+v, login %+v", item, item.Login)
	}
	if len(item.Login.URIs) != 1 || item.Login.URIs[0].URI != "https://valid.example.com" {
		t.Fatalf("expected only the URI with a valid checksum, got %+v", item.Login.URIs)
	}

	// An update keeps the item key and the fields the provider does not manage
	item.Name = "Renamed"
	updated, err := client.UpdateItem(ctx, *item)
	must(t, err)
	if updated.Name != "Renamed" || len(updated.PasswordHistory) != 1 {
		t.Fatalf("unexpected item after update: %+v", updated)
	}

	raw := getRawCipher(ctx, t, client, id)
	if raw.Key == nil || *raw.Key != encryptedItemKey {
		t.Fatalf("the update changed the item key")
	}
	if got := decrypt(t, raw.Name, itemKey); got != "Renamed" {
		t.Fatalf("name decrypted with the item key: %q", got)
	}
	if raw.Login.AutofillOnPageLoad == nil || !*raw.Login.AutofillOnPageLoad {
		t.Fatalf("the update removed autofillOnPageLoad")
	}
	if got := decrypt(t, raw.PasswordHistory[0].Password, itemKey); got != "older password" {
		t.Fatalf("password history decrypted with the item key: %q", got)
	}
}

// Items created by the provider have an item key, encrypted with the owner's key, and URIs with
// checksums, so that current Bitwarden clients show them.
func TestAccItemCreatedWithItemKey(t *testing.T) {
	ctx, client, _, userKey := itemTestSetup(t)

	org, err := client.CreateOrganization(ctx, models.Organization{Name: "Item keys", CollectionName: "Default"})
	must(t, err)
	t.Cleanup(func() { _ = client.DeleteOrganization(context.Background(), org.ID) })
	collection, err := client.CreateOrganizationCollection(ctx, org.ID, models.Collection{Name: "Items"})
	must(t, err)
	orgSecret, ok := client.OrganizationSecret(org.ID)
	if !ok {
		t.Fatalf("no key for organization %s", org.ID)
	}

	for _, tc := range []struct {
		name     string
		orgID    string
		ownerKey symmetrickey.Key
	}{
		{"personal", "", userKey},
		{"organization", org.ID, orgSecret.Key},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := models.Item{
				Type:           models.ItemTypeLogin,
				OrganizationID: tc.orgID,
				Name:           "Created " + tc.name,
				Login: &models.ItemLogin{
					Password: "password",
					URIs:     []models.ItemLoginURI{{URI: "https://a.example.com"}, {URI: "https://b.example.com"}},
				},
			}
			if tc.orgID != "" {
				item.CollectionIDs = []string{collection.ID}
			}
			created, err := client.CreateItem(ctx, item)
			must(t, err)
			t.Cleanup(func() { _ = client.DeleteItem(context.Background(), created.ID) })
			if !created.HasKey || created.OrganizationID != tc.orgID || len(created.Login.URIs) != 2 {
				t.Fatalf("unexpected item: %+v, login %+v", created, created.Login)
			}
			if tc.orgID != "" && !slices.Equal(created.CollectionIDs, []string{collection.ID}) {
				t.Fatalf("unexpected collections %v", created.CollectionIDs)
			}

			raw := getRawCipher(ctx, t, client, created.ID)
			if raw.Key == nil {
				t.Fatalf("item created without an item key")
			}
			rawItemKey, err := encryptedstring.NewFromEncryptedValue(*raw.Key)
			must(t, err)
			keyBytes, err := crypt.Decrypt(rawItemKey, &tc.ownerKey)
			must(t, err)
			itemKey, err := symmetrickey.NewFromRawBytes(keyBytes)
			must(t, err)

			if got := decrypt(t, raw.Name, *itemKey); got != item.Name {
				t.Fatalf("name decrypted with the item key: %q", got)
			}
			if got := decrypt(t, *raw.Login.Password, *itemKey); got != "password" {
				t.Fatalf("password decrypted with the item key: %q", got)
			}
			for _, uri := range raw.Login.URIs {
				plain := decrypt(t, uri.URI, *itemKey)
				if uri.URIChecksum == nil || decrypt(t, *uri.URIChecksum, *itemKey) != checksum(plain) {
					t.Fatalf("URI %s has no valid checksum", plain)
				}
			}
		})
	}
}

// Items in the trash are not found, and can be deleted.
func TestAccItemInTrash(t *testing.T) {
	ctx, client, _, _ := itemTestSetup(t)

	created, err := client.CreateItem(ctx, models.Item{Type: models.ItemTypeSecureNote, Name: "Trashed", Notes: "Note"})
	must(t, err)
	must(t, client.DoRequest(ctx, http.MethodPut, "/api/ciphers/"+created.ID+"/delete", nil, nil))

	if _, err := client.GetItem(ctx, created.ID); !vaultwarden.IsNotFound(err) {
		t.Fatalf("expected an item in the trash not to be found, got %v", err)
	}
	must(t, client.DeleteItem(ctx, created.ID))

	var raw json.RawMessage
	if err := client.DoRequest(ctx, http.MethodGet, "/api/ciphers/"+created.ID, nil, &raw); err == nil {
		t.Fatalf("item %s was not deleted", created.ID)
	}
	// Deleting it again succeeds
	must(t, client.DeleteItem(ctx, created.ID))
}
