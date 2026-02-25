package state

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// EncryptionKeyEnvVar is the environment variable for the state encryption key.
	EncryptionKeyEnvVar = "PICKLR_STATE_ENCRYPTION_KEY"

	// Encrypted state file header
	encryptedHeader = "# PICKLR_ENCRYPTED_STATE\n"
)

// EncryptState encrypts state content using AES-256-GCM with a key from the environment.
// Returns the original content if no encryption key is configured.
func EncryptState(content []byte) ([]byte, error) {
	key := getEncryptionKey()
	if key == nil {
		return content, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, content, nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	return []byte(encryptedHeader + encoded + "\n"), nil
}

// DecryptState decrypts state content if it's encrypted.
// Returns the original content if not encrypted.
func DecryptState(content []byte) ([]byte, error) {
	if !IsEncrypted(content) {
		return content, nil
	}

	key := getEncryptionKey()
	if key == nil {
		return nil, fmt.Errorf("state file is encrypted but %s is not set", EncryptionKeyEnvVar)
	}

	// Strip the header and decode
	encoded := strings.TrimPrefix(string(content), encryptedHeader)
	encoded = strings.TrimSpace(encoded)

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted state: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt state (wrong key?): %w", err)
	}

	return plaintext, nil
}

// IsEncrypted checks if state content is encrypted.
func IsEncrypted(content []byte) bool {
	return strings.HasPrefix(string(content), encryptedHeader)
}

// getEncryptionKey returns the 32-byte AES key from environment, or nil if not set.
func getEncryptionKey() []byte {
	keyStr := os.Getenv(EncryptionKeyEnvVar)
	if keyStr == "" {
		return nil
	}

	// Key must be exactly 32 bytes for AES-256
	// If shorter, pad with zeros; if longer, truncate
	key := make([]byte, 32)
	copy(key, []byte(keyStr))
	return key
}
