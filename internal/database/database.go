package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// ensureDir creates a directory if it doesn't exist
func ensureDir(dir string) error {
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

// New creates a new database connection
func New(databaseURL string) (*sql.DB, error) {
	// Ensure the directory exists
	dir := filepath.Dir(databaseURL)
	if err := ensureDir(dir); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// Migrate runs database migrations
func Migrate(db *sql.DB) error {
	migrations := []string{
		createUsersTable,
		createProjectsTable,
		createDeploymentsTable,
		createEnvironmentVariablesTable,
		createIndexes,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("failed to run migration: %w", err)
		}
	}

	return nil
}

const createUsersTable = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	github_id INTEGER UNIQUE NOT NULL,
	username TEXT NOT NULL,
	email TEXT,
	avatar_url TEXT,
	access_token TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createProjectsTable = `
CREATE TABLE IF NOT EXISTS projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	github_repo_id INTEGER NOT NULL,
	repo_url TEXT NOT NULL,
	branch TEXT NOT NULL DEFAULT 'main',
	subdomain TEXT UNIQUE NOT NULL,
	build_command TEXT DEFAULT 'go build -o main .',
	start_command TEXT DEFAULT './main',
	port INTEGER DEFAULT 8080,
	status TEXT DEFAULT 'inactive',
	last_deploy DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);`

const createDeploymentsTable = `
CREATE TABLE IF NOT EXISTS deployments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL,
	commit_sha TEXT NOT NULL,
	status TEXT DEFAULT 'pending',
	build_log TEXT,
	error_msg TEXT,
	started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	finished_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE
);`

const createEnvironmentVariablesTable = `
CREATE TABLE IF NOT EXISTS environment_variables (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL,
	key TEXT NOT NULL,
	value TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE,
	UNIQUE(project_id, key)
);`

const createIndexes = `
CREATE INDEX IF NOT EXISTS idx_projects_user_id ON projects(user_id);
CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);
CREATE INDEX IF NOT EXISTS idx_environment_variables_project_id ON environment_variables(project_id);
CREATE INDEX IF NOT EXISTS idx_projects_subdomain ON projects(subdomain);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
`
