package vaultwarden

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newRateLimitedServer returns a server that answers the first limited requests with HTTP 429
// and body, and later ones with `"ok"`. It records the request bodies it received.
func newRateLimitedServer(t *testing.T, limited int32, body string) (*httptest.Server, *atomic.Int32, *[]string) {
	t.Helper()
	var requests atomic.Int32
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if requests.Add(1) <= limited {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(body))
			return
		}
		_, _ = w.Write([]byte(`"ok"`))
	}))
	t.Cleanup(server.Close)
	return server, &requests, &bodies
}

func newTestClient(t *testing.T, endpoint string, maxRetryWait time.Duration) *Client {
	t.Helper()
	client, err := New(endpoint, WithAdminToken("token"), WithMaxRetryWait(maxRetryWait))
	if err != nil {
		t.Fatal(err)
	}
	client.retryBaseDelay = time.Millisecond
	return client
}

func TestSendRetriesRateLimitedRequests(t *testing.T) {
	server, requests, bodies := newRateLimitedServer(t, 3, `{"message":"Too many requests"}`)
	client := newTestClient(t, server.URL, time.Minute)

	var resp string
	if _, err := client.doUnauthenticatedRequest(context.Background(), http.MethodPost, "/identity/accounts/prelogin", map[string]string{"email": "a@example.com"}, &resp); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp != "ok" {
		t.Errorf("response = %q, want %q", resp, "ok")
	}
	if got := requests.Load(); got != 4 {
		t.Errorf("server got %d requests, want 4", got)
	}
	for i, body := range *bodies {
		if body != `{"email":"a@example.com"}` {
			t.Errorf("request %d body = %q, want the original body on every retry", i+1, body)
		}
	}
}

func TestSendGivesUpOnRateLimit(t *testing.T) {
	tests := []struct {
		name string
		body string
		hint string
	}{
		{"login", `{"message":"Too many login requests"}`, "LOGIN_RATELIMIT_SECONDS"},
		{"unauthenticated", `{"message":"Too many requests"}`, "UNAUTHENTICATED_RATELIMIT_SECONDS"},
		{"admin", `<!DOCTYPE html><html><h6>Error: Too many requests, try again later.</h6></html>`, "ADMIN_RATELIMIT_SECONDS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, requests, _ := newRateLimitedServer(t, 1000, tt.body)
			client := newTestClient(t, server.URL, 20*time.Millisecond)

			_, err := client.doUnauthenticatedRequest(context.Background(), http.MethodGet, "/api/version", nil, nil)
			if !hasStatus(err, http.StatusTooManyRequests) {
				t.Fatalf("error = %v, want an APIError with status 429", err)
			}
			if !strings.Contains(err.Error(), tt.hint) {
				t.Errorf("error = %q, want it to mention %s", err, tt.hint)
			}
			if got := requests.Load(); got < 2 {
				t.Errorf("server got %d requests, want retries", got)
			}
		})
	}
}

func TestSendWithoutRetries(t *testing.T) {
	server, requests, _ := newRateLimitedServer(t, 1000, `{"message":"Too many requests"}`)
	client := newTestClient(t, server.URL, 0)

	if _, err := client.doUnauthenticatedRequest(context.Background(), http.MethodGet, "/api/version", nil, nil); !hasStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("error = %v, want an APIError with status 429", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server got %d requests, want 1", got)
	}
}

func TestSendStopsRetryingWhenCanceled(t *testing.T) {
	server, _, _ := newRateLimitedServer(t, 1000, `{"message":"Too many requests"}`)
	client := newTestClient(t, server.URL, time.Hour)
	client.retryBaseDelay = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.doUnauthenticatedRequest(ctx, http.MethodGet, "/api/version", nil, nil)
	if !hasStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("error = %v, want an APIError with status 429", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("request returned after %s, want it to stop waiting when the context is canceled", elapsed)
	}
}

func TestSendDoesNotRetryOtherErrors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL, time.Minute)

	var apiErr *APIError
	if _, err := client.doUnauthenticatedRequest(context.Background(), http.MethodGet, "/api/version", nil, nil); !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("error = %v, want an APIError with status 400", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server got %d requests, want 1", got)
	}
}

func TestSendMultipartAndRawResponse(t *testing.T) {
	var contentType, fileName string
	var data []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload":
			contentType = r.Header.Get("Content-Type")
			file, header, err := r.FormFile("data")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			fileName = header.Filename
			data, _ = io.ReadAll(file)
		case "/attachments/item/file":
			if r.URL.Query().Get("token") != "abc" {
				http.Error(w, "bad token", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte{0, 1, 2, 255})
		}
	}))
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL, time.Minute)
	ctx := context.Background()

	if _, err := client.doUnauthenticatedRequest(ctx, http.MethodPost, "/upload", multipartFile{field: "data", fileName: "2.enc", data: []byte{9, 8, 7}}, nil); err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") || fileName != "2.enc" || string(data) != string([]byte{9, 8, 7}) {
		t.Errorf("unexpected upload: content type %q, file name %q, data %v", contentType, fileName, data)
	}

	// Relative and absolute URLs return the raw body
	for _, path := range []string{"/attachments/item/file?token=abc", server.URL + "/attachments/item/file?token=abc"} {
		var body []byte
		if _, err := client.doUnauthenticatedRequest(ctx, http.MethodGet, path, nil, &body); err != nil {
			t.Fatalf("download from %s failed: %v", path, err)
		}
		if string(body) != string([]byte{0, 1, 2, 255}) {
			t.Errorf("download from %s returned %v", path, body)
		}
	}
}

func TestAttachmentDownloadPath(t *testing.T) {
	for downloadURL, want := range map[string]string{
		// Vaultwarden's own storage, behind another DOMAIN or under a path
		"http://localhost/attachments/item/file?token=abc":         "/attachments/item/file?token=abc",
		"https://vw.example.com/vw/attachments/item/file?token=ab": "/attachments/item/file?token=ab",
		// Object storage
		"https://bucket.s3.example.com/item/file?X-Amz-Signature=1": "https://bucket.s3.example.com/item/file?X-Amz-Signature=1",
		"https://bucket.s3.example.com/attachments/other/file":      "https://bucket.s3.example.com/attachments/other/file",
	} {
		if got := attachmentDownloadPath(downloadURL, "item"); got != want {
			t.Errorf("attachmentDownloadPath(%q) = %q, want %q", downloadURL, got, want)
		}
	}
}
