package state

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt_NoKey(t *testing.T) {
	// Without env var, encryption is a no-op
	os.Unsetenv(EncryptionKeyEnvVar)

	content := []byte("version = 1\nserial = 0\n")
	encrypted, err := EncryptState(content)
	require.NoError(t, err)
	assert.Equal(t, content, encrypted) // Should be unchanged

	decrypted, err := DecryptState(content)
	require.NoError(t, err)
	assert.Equal(t, content, decrypted)
}

func TestEncryptDecrypt_WithKey(t *testing.T) {
	os.Setenv(EncryptionKeyEnvVar, "my-super-secret-encryption-key!!")
	defer os.Unsetenv(EncryptionKeyEnvVar)

	content := []byte("version = 1\nserial = 42\nlineage = \"test-uuid\"\n")

	encrypted, err := EncryptState(content)
	require.NoError(t, err)
	assert.NotEqual(t, content, encrypted)
	assert.True(t, IsEncrypted(encrypted))

	decrypted, err := DecryptState(encrypted)
	require.NoError(t, err)
	assert.Equal(t, content, decrypted)
}

func TestIsEncrypted(t *testing.T) {
	assert.True(t, IsEncrypted([]byte("# PICKLR_ENCRYPTED_STATE\nbase64data")))
	assert.False(t, IsEncrypted([]byte("version = 1\n")))
	assert.False(t, IsEncrypted([]byte("")))
}

func TestDecryptState_WrongKey(t *testing.T) {
	os.Setenv(EncryptionKeyEnvVar, "correct-key-for-encryption!!!!!")
	defer os.Unsetenv(EncryptionKeyEnvVar)

	content := []byte("test data")
	encrypted, err := EncryptState(content)
	require.NoError(t, err)

	// Try decrypting with wrong key
	os.Setenv(EncryptionKeyEnvVar, "wrong-key-for-decryption!!!!!!!")
	_, err = DecryptState(encrypted)
	assert.Error(t, err)
}

func TestDecryptState_NoKey(t *testing.T) {
	os.Setenv(EncryptionKeyEnvVar, "some-key-for-testing!!!!!!!!!!!!")
	defer os.Unsetenv(EncryptionKeyEnvVar)

	content := []byte("test data")
	encrypted, err := EncryptState(content)
	require.NoError(t, err)

	// Try decrypting without key
	os.Unsetenv(EncryptionKeyEnvVar)
	_, err = DecryptState(encrypted)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not set")
}
