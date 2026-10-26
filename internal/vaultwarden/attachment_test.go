package vaultwarden_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"testing"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/encryptedstring"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
)

// rawAttachment is an attachment as returned by Vaultwarden.
type rawAttachment struct {
	URL      string  `json:"url"`
	FileName string  `json:"fileName"`
	Key      *string `json:"key"`
}

// checkAttachment checks an attachment the way Bitwarden clients read it: its file name and key
// decrypt with itemKey, and the downloaded file with the attachment key.
func checkAttachment(ctx context.Context, t *testing.T, client *vaultwarden.Client, itemID, attachmentID string, itemKey symmetrickey.Key, fileName string, content []byte) {
	t.Helper()
	var raw rawAttachment
	must(t, client.DoRequest(ctx, http.MethodGet, "/api/ciphers/"+itemID+"/attachment/"+attachmentID, nil, &raw))
	if got := decrypt(t, raw.FileName, itemKey); got != fileName {
		t.Fatalf("file name decrypted with the item key: %q", got)
	}
	if raw.Key == nil {
		t.Fatalf("the attachment has no key")
	}
	attachmentKey, err := symmetrickey.NewFromRawBytes([]byte(decrypt(t, *raw.Key, itemKey)))
	must(t, err)

	got, err := client.GetAttachmentContent(ctx, itemID, attachmentID)
	must(t, err)
	if !bytes.Equal(got, content) {
		t.Fatalf("downloaded content differs: %q", got)
	}

	// The provider's download does not depend on its own encryption: check the encrypted buffer
	// stored on the server with the independently decrypted attachment key
	var buffer []byte
	must(t, client.DoRequest(ctx, http.MethodGet, attachmentPath(t, raw.URL), nil, &buffer))
	encrypted, err := encryptedstring.NewFromEncryptedBuffer(buffer)
	must(t, err)
	decrypted, err := crypt.Decrypt(encrypted, attachmentKey)
	must(t, err)
	if !bytes.Equal(decrypted, content) {
		t.Fatalf("content decrypted with the attachment key differs: %q", decrypted)
	}
}

// attachmentPath returns the path of a download URL, relative to the server.
func attachmentPath(t *testing.T, downloadURL string) string {
	t.Helper()
	i := bytes.Index([]byte(downloadURL), []byte("/attachments/"))
	if i < 0 {
		t.Fatalf("unexpected download URL %q", downloadURL)
	}
	return downloadURL[i:]
}

// An attachment of an item with an item key is encrypted with an attachment key, which is
// encrypted with the item key. Updating the item keeps the attachment readable.
func TestAccAttachmentWithItemKey(t *testing.T) {
	ctx, client, _, userKey := itemTestSetup(t)

	item, err := client.CreateItem(ctx, models.Item{Type: models.ItemTypeSecureNote, Name: "With attachment"})
	must(t, err)
	t.Cleanup(func() { _ = client.DeleteItem(context.Background(), item.ID) })

	content := make([]byte, 100_000)
	_, err = rand.Read(content)
	must(t, err)
	attachment, err := client.CreateAttachment(ctx, item.ID, "random.bin", content)
	must(t, err)
	if attachment.FileName != "random.bin" || attachment.Size < int64(len(content)) || attachment.SizeName == "" {
		t.Fatalf("unexpected attachment: %+v", attachment)
	}

	raw := getRawCipher(ctx, t, client, item.ID)
	if raw.Key == nil {
		t.Fatalf("the item has no item key")
	}
	itemKey, err := symmetrickey.NewFromRawBytes([]byte(decrypt(t, *raw.Key, userKey)))
	must(t, err)
	checkAttachment(ctx, t, client, item.ID, attachment.ID, *itemKey, "random.bin", content)

	item.Notes = "Updated"
	updated, err := client.UpdateItem(ctx, *item)
	must(t, err)
	if len(updated.Attachments) != 1 || updated.Attachments[0].ID != attachment.ID {
		t.Fatalf("unexpected attachments after the update: %+v", updated.Attachments)
	}
	checkAttachment(ctx, t, client, item.ID, attachment.ID, *itemKey, "random.bin", content)

	must(t, client.DeleteAttachment(ctx, item.ID, attachment.ID))
	if _, err := client.GetAttachment(ctx, item.ID, attachment.ID); !vaultwarden.IsNotFound(err) {
		t.Fatalf("the attachment was not deleted: %v", err)
	}
	if _, err := client.GetAttachmentContent(ctx, item.ID, attachment.ID); !vaultwarden.IsNotFound(err) {
		t.Fatalf("downloading a deleted attachment: %v", err)
	}
	must(t, client.DeleteAttachment(ctx, item.ID, attachment.ID))
}

// An attachment of an item without an item key, as older Bitwarden clients create them, has its
// file name and attachment key encrypted with the user key.
func TestAccAttachmentWithoutItemKey(t *testing.T) {
	ctx, client, userID, userKey := itemTestSetup(t)

	id := postCipher(ctx, t, client, map[string]any{
		"type":         2,
		"encryptedFor": userID,
		"name":         encrypt(t, "Keyless with attachment", userKey),
		"secureNote":   map[string]int{"type": 0},
	})

	content := []byte("hello, attachment")
	attachment, err := client.CreateAttachment(ctx, id, "hello.txt", content)
	must(t, err)
	checkAttachment(ctx, t, client, id, attachment.ID, userKey, "hello.txt", content)

	// Missing items and attachments are not found
	if _, err := client.GetAttachment(ctx, id, "00000000-0000-0000-0000-000000000000"); !vaultwarden.IsNotFound(err) {
		t.Fatalf("missing attachment: %v", err)
	}
	if _, err := client.CreateAttachment(ctx, "00000000-0000-0000-0000-000000000000", "a.txt", content); !vaultwarden.IsNotFound(err) {
		t.Fatalf("attachment of a missing item: %v", err)
	}
	if _, err := client.CreateAttachment(ctx, id, "empty.txt", nil); err == nil {
		t.Fatalf("an empty attachment was created")
	}
}
