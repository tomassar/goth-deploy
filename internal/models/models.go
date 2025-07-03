package models

import (
	"time"
)

// User represents a user account
type User struct {
	ID          int64     `json:"id" db:"id"`
	GitHubID    int64     `json:"github_id" db:"github_id"`
	Username    string    `json:"username" db:"username"`
	Email       string    `json:"email" db:"email"`
	AvatarURL   string    `json:"avatar_url" db:"avatar_url"`
	AccessToken string    `json:"-" db:"access_token"` // Don't serialize to JSON
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Project represents a GitHub repository that can be deployed
type Project struct {
	ID           int64      `json:"id" db:"id"`
	UserID       int64      `json:"user_id" db:"user_id"`
	Name         string     `json:"name" db:"name"`
	GitHubRepoID int64      `json:"github_repo_id" db:"github_repo_id"`
	RepoURL      string     `json:"repo_url" db:"repo_url"`
	Branch       string     `json:"branch" db:"branch"`
	Subdomain    string     `json:"subdomain" db:"subdomain"`
	BuildCommand string     `json:"build_command" db:"build_command"`
	StartCommand string     `json:"start_command" db:"start_command"`
	Port         int        `json:"port" db:"port"`
	Status       string     `json:"status" db:"status"` // active, inactive, building, failed
	LastDeploy   *time.Time `json:"last_deploy" db:"last_deploy"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

// Deployment represents a single deployment of a project
type Deployment struct {
	ID         int64      `json:"id" db:"id"`
	ProjectID  int64      `json:"project_id" db:"project_id"`
	CommitSHA  string     `json:"commit_sha" db:"commit_sha"`
	Status     string     `json:"status" db:"status"` // pending, building, success, failed
	BuildLog   string     `json:"build_log" db:"build_log"`
	ErrorMsg   string     `json:"error_msg" db:"error_msg"`
	StartedAt  time.Time  `json:"started_at" db:"started_at"`
	FinishedAt *time.Time `json:"finished_at" db:"finished_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
}

// EnvironmentVariable represents an environment variable for a project
type EnvironmentVariable struct {
	ID        int64     `json:"id" db:"id"`
	ProjectID int64     `json:"project_id" db:"project_id"`
	Key       string    `json:"key" db:"key"`
	Value     string    `json:"value" db:"value"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// DeploymentStatus constants
const (
	StatusPending  = "pending"
	StatusBuilding = "building"
	StatusSuccess  = "success"
	StatusFailed   = "failed"
)

// ProjectStatus constants
const (
	ProjectStatusActive   = "active"
	ProjectStatusInactive = "inactive"
	ProjectStatusBuilding = "building"
	ProjectStatusFailed   = "failed"
)

// GitHubRepo represents a repository from GitHub API
type GitHubRepo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	HTMLURL       string `json:"html_url"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	Language      string `json:"language"`
	DefaultBranch string `json:"default_branch"`
}
