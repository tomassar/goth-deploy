package database

import (
	"database/sql"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
)

func Connect(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", databaseURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

func Migrate(databaseURL string) error {
	db, err := sql.Open("sqlite3", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return err
	}

	sourceURL := "file://" + filepath.Join("migrations")
	source, err := (&file.File{}).Open(sourceURL)
	if err != nil {
		// If migrations directory doesn't exist, create tables directly
		return createTables(db)
	}

	m, err := migrate.NewWithInstance("file", source, "sqlite3", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			github_id INTEGER UNIQUE NOT NULL,
			username TEXT NOT NULL,
			email TEXT,
			avatar_url TEXT,
			access_token TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			repository TEXT NOT NULL,
			branch TEXT DEFAULT 'main',
			subdomain TEXT UNIQUE NOT NULL,
			status TEXT DEFAULT 'idle',
			build_logs TEXT,
			last_deploy_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS deployments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			status TEXT DEFAULT 'pending',
			commit_sha TEXT,
			logs TEXT,
			error_message TEXT,
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			ended_at DATETIME,
			FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE
		);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}

	// Add new columns to existing tables if they don't exist
	alterQueries := []string{
		`ALTER TABLE projects ADD COLUMN build_logs TEXT DEFAULT '';`,
		`ALTER TABLE deployments ADD COLUMN error_message TEXT DEFAULT '';`,
	}

	for _, query := range alterQueries {
		db.Exec(query) // Ignore errors for existing columns
	}

	return nil
}
