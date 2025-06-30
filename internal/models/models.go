package models

import (
	"time"
)

type User struct {
	ID          int       `json:"id" db:"id"`
	GitHubID    int       `json:"github_id" db:"github_id"`
	Username    string    `json:"username" db:"username"`
	Email       string    `json:"email" db:"email"`
	AvatarURL   string    `json:"avatar_url" db:"avatar_url"`
	AccessToken string    `json:"-" db:"access_token"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Project struct {
	ID           int        `json:"id" db:"id"`
	UserID       int        `json:"user_id" db:"user_id"`
	Name         string     `json:"name" db:"name"`
	Repository   string     `json:"repository" db:"repository"`
	Branch       string     `json:"branch" db:"branch"`
	Subdomain    string     `json:"subdomain" db:"subdomain"`
	Status       string     `json:"status" db:"status"`
	BuildLogs    string     `json:"build_logs" db:"build_logs"`
	LastDeployAt *time.Time `json:"last_deploy_at" db:"last_deploy_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

type Deployment struct {
	ID           int        `json:"id" db:"id"`
	ProjectID    int        `json:"project_id" db:"project_id"`
	Status       string     `json:"status" db:"status"`
	CommitSHA    string     `json:"commit_sha" db:"commit_sha"`
	Logs         string     `json:"logs" db:"logs"`
	ErrorMessage string     `json:"error_message" db:"error_message"`
	StartedAt    time.Time  `json:"started_at" db:"started_at"`
	EndedAt      *time.Time `json:"ended_at" db:"ended_at"`
}

type Secret struct {
	ID             int       `json:"id" db:"id"`
	ProjectID      int       `json:"project_id" db:"project_id"`
	KeyName        string    `json:"key_name" db:"key_name"`
	EncryptedValue string    `json:"-" db:"encrypted_value"` // Never expose encrypted value in JSON
	Description    string    `json:"description" db:"description"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// SecretInput represents input when creating/updating secrets
type SecretInput struct {
	KeyName     string `json:"key_name"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// SecretDisplay represents how secrets are displayed (masked value)
type SecretDisplay struct {
	ID          int       `json:"id"`
	ProjectID   int       `json:"project_id"`
	KeyName     string    `json:"key_name"`
	MaskedValue string    `json:"masked_value"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const (
	ProjectStatusIdle     = "idle"
	ProjectStatusBuilding = "building"
	ProjectStatusActive   = "active"
	ProjectStatusFailed   = "failed"
)

const (
	DeploymentStatusPending = "pending"
	DeploymentStatusRunning = "running"
	DeploymentStatusSuccess = "success"
	DeploymentStatusFailed  = "failed"
)
