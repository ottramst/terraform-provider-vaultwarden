package test

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"os"
	"strings"
	"sync"
	"testing"
)

const (
	TestPassword   = "test-password-123!"
	TestEmail      = "test@example.com"
	TestAdminToken = "admin_token"
)

var (
	testClient      *vaultwarden.Client
	testClientOnce  sync.Once
	testClientError error
	hashedPassword  string
	TestBaseURL     = GetTestBaseURL()
)

// GetTestBaseURL returns the base URL for the Vaultwarden instance
// It can be configured via the VW_TEST_URL environment variable
func GetTestBaseURL() string {
	if url := os.Getenv("VW_TEST_URL"); url != "" {
		return url
	}
	// Default to docker service name when running in container
	return "http://tf-vaultwarden:8000"
}

// GetTestClient returns a singleton test client, creating it if necessary
func GetTestClient(ctx context.Context, t *testing.T) (*vaultwarden.Client, error) {
	t.Logf("Getting test client for email: %s", TestEmail)
	testClientOnce.Do(func() {
		t.Log("Initializing test client for the first time")
		testClient, hashedPassword, testClientError = createTestAccount(ctx, t)
		if testClientError != nil {
			t.Logf("Failed to create test account: %v", testClientError)
		} else {
			t.Logf("Successfully created test account")
		}
	})
	return testClient, testClientError
}

// createTestAccount creates a test user account for acceptance tests
func createTestAccount(ctx context.Context, t *testing.T) (*vaultwarden.Client, string, error) {
	t.Logf("Creating test account with email: %s at %s", TestEmail, TestBaseURL)

	// Create client with credentials and admin token
	client, err := vaultwarden.New(
		TestBaseURL,
		vaultwarden.WithUserCredentials(TestEmail, TestPassword),
		vaultwarden.WithAdminToken(TestAdminToken),
	)
	if err != nil {
		t.Logf("Failed to create client: %v", err)
		return nil, "", fmt.Errorf("failed to create client: %w", err)
	}

	// Do prelogin to get KDF parameters
	preloginResp, err := client.PreLogin(ctx)
	if err != nil {
		t.Logf("Prelogin failed: %v", err)
		return nil, "", fmt.Errorf("prelogin failed: %w", err)
	}
	t.Log("Prelogin successful")

	// Build the KDF configuration
	kdfConfig := &models.KdfConfiguration{
		KdfType:        preloginResp.Kdf,
		KdfIterations:  preloginResp.KdfIterations,
		KdfMemory:      preloginResp.KdfMemory,
		KdfParallelism: preloginResp.KdfParallelism,
	}

	// Build prelogin key
	preloginKey, err := keybuilder.BuildPreloginKey(TestPassword, TestEmail, kdfConfig)
	if err != nil {
		t.Logf("Failed to build prelogin key: %v", err)
		return nil, "", fmt.Errorf("failed to build prelogin key: %w", err)
	}
	t.Log("Prelogin key built")

	// Hash password
	hashedPw := crypt.HashPassword(TestPassword, *preloginKey, false)

	// Try to log in first - if successful, the user already exists
	if _, err := client.LoginWithUserCredentials(ctx, hashedPw); err == nil {
		t.Log("User already exists")
		return client, hashedPw, nil
	}
	t.Log("User does not exist")

	// Create encryption key
	encryptionKey, encryptedEncryptionKey, err := keybuilder.GenerateEncryptionKey(*preloginKey)
	if err != nil {
		t.Logf("Failed to generate encryption key: %v", err)
		return nil, "", fmt.Errorf("failed to generate encryption key: %w", err)
	}
	t.Log("Encryption key generated")

	// Generate public/private key pair
	publicKey, encryptedPrivateKey, err := keybuilder.GenerateEncryptedRSAKeyPair(*encryptionKey)
	if err != nil {
		t.Logf("Failed to generate RSA key pair: %v", err)
		return nil, "", fmt.Errorf("failed to generate RSA key pair: %w", err)
	}
	t.Log("RSA key pair generated")

	// Create registration request
	signupReq := vaultwarden.RegisterUserRequest{
		Email:              TestEmail,
		MasterPasswordHash: hashedPw,
		Key:                encryptedEncryptionKey,
		Kdf:                kdfConfig.KdfType,
		KdfIterations:      kdfConfig.KdfIterations,
		KdfMemory:          kdfConfig.KdfMemory,
		KdfParallelism:     kdfConfig.KdfParallelism,
		Keys: models.KeyPair{
			PublicKey:           publicKey,
			EncryptedPrivateKey: encryptedPrivateKey,
		},
	}

	// Register user
	if err := client.RegisterUser(ctx, signupReq); err != nil {
		if !isUserExistsError(err) {
			t.Logf("Failed to register user: %v", err)
			return nil, "", fmt.Errorf("failed to register user: %w", err)
		}
		t.Log("User already exists")
	}
	t.Log("User registered")

	// Login after registration to ensure everything works
	if _, err := client.LoginWithUserCredentials(ctx, hashedPw); err != nil {
		return nil, "", fmt.Errorf("failed to login after registration: %w", err)
	}

	return client, hashedPw, nil
}

// LoginTestClient performs a login with the test client
func LoginTestClient(ctx context.Context, t *testing.T) error {
	client, err := GetTestClient(ctx, t)
	if err != nil {
		return fmt.Errorf("failed to get test client: %w", err)
	}

	if _, err := client.LoginWithUserCredentials(ctx, hashedPassword); err != nil {
		return fmt.Errorf("failed to login with test account: %w", err)
	}

	return nil
}

// isUserExistsError checks if the error indicates the user already exists
func isUserExistsError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "user already exists") ||
		strings.Contains(err.Error(), "email already taken") ||
		strings.Contains(err.Error(), "registration not allowed"))
}
