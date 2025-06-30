package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"deployer/internal/models"
)

type SecretsService struct {
	db            *sql.DB
	encryptionKey []byte
}

func NewSecretsService(db *sql.DB, encryptionKey string) *SecretsService {
	// Use a 32-byte key for AES-256
	key := make([]byte, 32)
	copy(key, []byte(encryptionKey))

	return &SecretsService{
		db:            db,
		encryptionKey: key,
	}
}

// encrypt encrypts a plaintext string using AES-GCM
func (s *SecretsService) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts a base64-encoded ciphertext using AES-GCM
func (s *SecretsService) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(data) < gcm.NonceSize() {
		return "", errors.New("malformed ciphertext")
	}

	nonce, cipherData := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// maskValue masks a secret value for display
func (s *SecretsService) maskValue(value string) string {
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}

// CreateSecret creates a new encrypted secret
func (s *SecretsService) CreateSecret(projectID int, input models.SecretInput) error {
	// Validate input
	if input.KeyName == "" {
		return errors.New("key name is required")
	}
	if input.Value == "" {
		return errors.New("value is required")
	}

	// Encrypt the value
	encryptedValue, err := s.encrypt(input.Value)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// Insert into database
	_, err = s.db.Exec(`
		INSERT INTO secrets (project_id, key_name, encrypted_value, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, projectID, input.KeyName, encryptedValue, input.Description)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("secret with key '%s' already exists", input.KeyName)
		}
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

// GetProjectSecrets returns all secrets for a project (with masked values)
func (s *SecretsService) GetProjectSecrets(projectID int) ([]models.SecretDisplay, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, key_name, encrypted_value, description, created_at, updated_at
		FROM secrets WHERE project_id = ? ORDER BY key_name ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []models.SecretDisplay
	for rows.Next() {
		var secret models.Secret
		err := rows.Scan(&secret.ID, &secret.ProjectID, &secret.KeyName, &secret.EncryptedValue,
			&secret.Description, &secret.CreatedAt, &secret.UpdatedAt)
		if err != nil {
			return nil, err
		}

		// Decrypt to get original length for masking
		decryptedValue, err := s.decrypt(secret.EncryptedValue)
		if err != nil {
			// If decryption fails, use a default mask
			decryptedValue = "********"
		}

		secrets = append(secrets, models.SecretDisplay{
			ID:          secret.ID,
			ProjectID:   secret.ProjectID,
			KeyName:     secret.KeyName,
			MaskedValue: s.maskValue(decryptedValue),
			Description: secret.Description,
			CreatedAt:   secret.CreatedAt,
			UpdatedAt:   secret.UpdatedAt,
		})
	}

	return secrets, nil
}

// GetSecretValue returns the decrypted value of a specific secret
func (s *SecretsService) GetSecretValue(secretID int, projectID int) (string, error) {
	var encryptedValue string
	err := s.db.QueryRow(`
		SELECT encrypted_value FROM secrets WHERE id = ? AND project_id = ?
	`, secretID, projectID).Scan(&encryptedValue)

	if err != nil {
		return "", err
	}

	return s.decrypt(encryptedValue)
}

// UpdateSecret updates an existing secret
func (s *SecretsService) UpdateSecret(secretID int, projectID int, input models.SecretInput) error {
	// Validate input
	if input.KeyName == "" {
		return errors.New("key name is required")
	}
	if input.Value == "" {
		return errors.New("value is required")
	}

	// Encrypt the new value
	encryptedValue, err := s.encrypt(input.Value)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// Update in database
	result, err := s.db.Exec(`
		UPDATE secrets SET key_name = ?, encrypted_value = ?, description = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND project_id = ?
	`, input.KeyName, encryptedValue, input.Description, secretID, projectID)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("secret with key '%s' already exists", input.KeyName)
		}
		return fmt.Errorf("failed to update secret: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("secret not found")
	}

	return nil
}

// DeleteSecret deletes a secret
func (s *SecretsService) DeleteSecret(secretID int, projectID int) error {
	result, err := s.db.Exec(`
		DELETE FROM secrets WHERE id = ? AND project_id = ?
	`, secretID, projectID)

	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("secret not found")
	}

	return nil
}

// GetProjectSecretsForDeployment returns all secrets for a project as environment variables (decrypted)
func (s *SecretsService) GetProjectSecretsForDeployment(projectID int) (map[string]string, error) {
	rows, err := s.db.Query(`
		SELECT key_name, encrypted_value FROM secrets WHERE project_id = ?
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	secrets := make(map[string]string)
	for rows.Next() {
		var keyName, encryptedValue string
		err := rows.Scan(&keyName, &encryptedValue)
		if err != nil {
			return nil, err
		}

		decryptedValue, err := s.decrypt(encryptedValue)
		if err != nil {
			// Log error but continue with other secrets
			fmt.Printf("Failed to decrypt secret %s: %v\n", keyName, err)
			continue
		}

		secrets[keyName] = decryptedValue
	}

	return secrets, nil
}
