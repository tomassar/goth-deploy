# Secrets Management in GoTH Deployer

## Overview

GoTH Deployer implements a secure secrets management system that encrypts sensitive environment variables at rest and injects them into deployed applications at runtime.

## Architecture

### 1. **Encryption at Rest**
- **Algorithm**: AES-256-GCM encryption
- **Key Management**: Uses a configurable encryption key from `ENCRYPTION_KEY` environment variable
- **Storage**: Encrypted values stored in SQLite database (`secrets` table)

```sql
CREATE TABLE secrets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    key_name TEXT NOT NULL,
    encrypted_value TEXT NOT NULL,  -- AES-256-GCM encrypted
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, key_name)
);
```

### 2. **Encryption Service**
Located in `internal/services/secrets.go`:

- **`encrypt(plaintext)`**: Encrypts values using AES-GCM with random nonce
- **`decrypt(ciphertext)`**: Decrypts values for runtime use
- **`CreateSecret()`**: Stores new encrypted secrets
- **`GetProjectSecretsForDeployment()`**: Returns decrypted secrets for deployment

### 3. **Runtime Injection**
During deployment (`internal/services/deployment.go`):

```go
// Get decrypted secrets for the project
secrets, err := s.secretsService.GetProjectSecretsForDeployment(projectID)

// Convert to environment variables
for key, value := range secrets {
    env = append(env, fmt.Sprintf("%s=%s", key, value))
}

// Start process with secrets as environment variables
cmd.Env = append(os.Environ(), env...)
```

## Security Features

### 🔐 **Encryption**
- **AES-256-GCM**: Industry-standard authenticated encryption
- **Random Nonces**: Each encryption uses a unique nonce
- **Base64 Encoding**: Encrypted data is base64-encoded for storage

### 🎭 **Display Masking**
- Secrets are masked in the UI (e.g., `****abcd`)
- Full values only shown when explicitly requested
- Automatic re-masking after viewing

### 🔑 **Access Control**
- Secrets are project-scoped (isolated per project)
- Only accessible during deployment by the deployment service
- No API endpoints expose raw encrypted values

### 🚫 **Zero-Trust Design**
- Secrets never appear in logs or build output
- Database stores only encrypted values
- Decryption only happens at deployment time

## Configuration

### Environment Variables
```bash
# Production: Use a strong, random 32-byte key
ENCRYPTION_KEY=your-secure-32-byte-encryption-key-here

# Development: Uses default key (change for production!)
ENCRYPTION_KEY=default-encryption-key-change-me-in-production
```

### Best Practices
1. **Rotate encryption keys** periodically in production
2. **Use strong encryption keys** (32+ random bytes)
3. **Backup encrypted secrets** along with encryption keys
4. **Monitor secret access** through application logs

## Usage Workflow

### 1. **Adding Secrets**
```bash
# Via Web UI:
Project Details → Add Variable → Enter Key/Value → Save
```

### 2. **Deployment Integration**
```go
// Your GoTH app automatically receives secrets as environment variables:
dbURL := os.Getenv("DATABASE_URL")
apiKey := os.Getenv("API_KEY")
```

### 3. **Managing Secrets**
- **Create**: Add new environment variables
- **Read**: View masked values, reveal on demand
- **Update**: Modify existing secret values
- **Delete**: Remove secrets (with confirmation)

## Technical Implementation

### Encryption Flow
```
Plaintext → AES-256-GCM → Base64 → SQLite Database
```

### Deployment Flow
```
Database → Base64 Decode → AES-256-GCM Decrypt → Environment Variables → Process
```

### API Endpoints
- `GET /projects/{id}/secrets` - List project secrets (masked)
- `POST /projects/{id}/secrets` - Create new secret
- `PUT /projects/{id}/secrets/{secretId}` - Update secret
- `DELETE /projects/{id}/secrets/{secretId}` - Delete secret
- `GET /projects/{id}/secrets/{secretId}/value` - Get decrypted value

## Security Considerations

### ✅ **Secure**
- Encryption at rest with AES-256-GCM
- Project-isolated secret storage
- No plaintext secrets in database
- Secure environment variable injection

### 🚨 **CRITICAL SECURITY WARNING**
**This system is currently NOT SECURE for production use with untrusted code.**

- **No Process Isolation**: All apps run with same privileges as deployer
- **Full System Access**: Malicious apps can access other projects, database, and system files
- **Environment Leakage**: Apps inherit sensitive environment variables like `ENCRYPTION_KEY`
- **No Resource Limits**: Apps can consume unlimited CPU/memory/disk

**See `SECURITY-ANALYSIS.md` for detailed security analysis and recommendations.**

### ⚠️ **Important Notes**
- **Change default encryption key** in production
- **Backup encryption keys** securely
- **Secrets are visible** to deployed applications as environment variables
- **SQLite database** should be protected with appropriate file permissions
- **Only use with trusted code** in controlled environments

## Example Usage

```bash
# Add secrets via UI
DATABASE_URL=postgresql://user:pass@host:5432/db
API_KEY=sk-1234567890abcdef
STRIPE_SECRET=sk_test_...

# Access in your GoTH application
package main

import (
    "os"
    "database/sql"
)

func main() {
    // Secrets automatically available as environment variables
    dbURL := os.Getenv("DATABASE_URL")
    apiKey := os.Getenv("API_KEY")
    
    // Use secrets in your application
    db, err := sql.Open("postgres", dbURL)
    // ...
}
```

---

This secrets management system provides enterprise-grade security for sensitive configuration while maintaining simplicity for developers. 