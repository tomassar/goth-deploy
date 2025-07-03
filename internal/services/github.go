package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"goth-deploy/internal/models"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
	githubOAuth "golang.org/x/oauth2/github"
)

// GitHubService handles GitHub OAuth and API interactions
type GitHubService struct {
	Config *oauth2.Config
}

// NewGitHubService creates a new GitHub service
func NewGitHubService(clientID, clientSecret, redirectURL string) *GitHubService {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"read:user", "user:email", "repo"},
		Endpoint:     githubOAuth.Endpoint,
	}

	return &GitHubService{
		Config: config,
	}
}

// GetAuthURL returns the GitHub OAuth authorization URL
func (g *GitHubService) GetAuthURL(state string) string {
	return g.Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ExchangeCode exchanges the authorization code for an access token
func (g *GitHubService) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	return g.Config.Exchange(ctx, code)
}

// GetUserInfo retrieves user information from GitHub API
func (g *GitHubService) GetUserInfo(ctx context.Context, token *oauth2.Token) (*models.User, error) {
	client := g.Config.Client(ctx, token)
	githubClient := github.NewClient(client)

	// Get user info
	user, _, err := githubClient.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Get user emails
	emails, _, err := githubClient.Users.ListEmails(ctx, nil)
	if err != nil {
		log.Printf("Failed to get user emails: %v", err)
	}

	var primaryEmail string
	for _, email := range emails {
		if email.GetPrimary() {
			primaryEmail = email.GetEmail()
			break
		}
	}

	return &models.User{
		GitHubID:    user.GetID(),
		Username:    user.GetLogin(),
		Email:       primaryEmail,
		AvatarURL:   user.GetAvatarURL(),
		AccessToken: token.AccessToken,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// GetUserRepositories retrieves user's repositories from GitHub
func (g *GitHubService) GetUserRepositories(ctx context.Context, accessToken string) ([]models.GitHubRepo, error) {
	token := &oauth2.Token{AccessToken: accessToken}
	client := g.Config.Client(ctx, token)
	githubClient := github.NewClient(client)

	var allRepos []models.GitHubRepo
	repoMap := make(map[int64]bool) // To avoid duplicates

	// Get user's own repositories and repositories they collaborate on
	opts := &github.RepositoryListOptions{
		Visibility:  "all",
		Affiliation: "owner,collaborator,organization_member",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	log.Printf("Fetching user repositories...")
	for {
		repos, resp, err := githubClient.Repositories.List(ctx, "", opts)
		if err != nil {
			log.Printf("Error fetching repositories: %v", err)
			return nil, fmt.Errorf("failed to get repositories: %w", err)
		}

		log.Printf("Retrieved %d repositories in this batch", len(repos))

		for _, repo := range repos {
			// Skip if we already have this repository (avoid duplicates)
			if repoMap[repo.GetID()] {
				continue
			}
			repoMap[repo.GetID()] = true

			// Include all repositories - user should be able to deploy any project
			allRepos = append(allRepos, models.GitHubRepo{
				ID:            repo.GetID(),
				Name:          repo.GetName(),
				FullName:      repo.GetFullName(),
				CloneURL:      repo.GetCloneURL(),
				HTMLURL:       repo.GetHTMLURL(),
				Description:   repo.GetDescription(),
				Private:       repo.GetPrivate(),
				Language:      repo.GetLanguage(),
				DefaultBranch: repo.GetDefaultBranch(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	log.Printf("Total repositories fetched: %d", len(allRepos))
	return allRepos, nil
}

// CreateOrUpdateUser creates or updates a user in the database
func (g *GitHubService) CreateOrUpdateUser(db *sql.DB, user *models.User) error {
	// Check if user exists
	var existingID int64
	err := db.QueryRow("SELECT id FROM users WHERE github_id = ?", user.GitHubID).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Create new user
		result, err := db.Exec(`
			INSERT INTO users (github_id, username, email, avatar_url, access_token, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, user.GitHubID, user.Username, user.Email, user.AvatarURL, user.AccessToken, user.CreatedAt, user.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get user ID: %w", err)
		}
		user.ID = id
	} else if err != nil {
		return fmt.Errorf("failed to check existing user: %w", err)
	} else {
		// Update existing user
		user.ID = existingID
		user.UpdatedAt = time.Now()
		_, err = db.Exec(`
			UPDATE users 
			SET username = ?, email = ?, avatar_url = ?, access_token = ?, updated_at = ?
			WHERE github_id = ?
		`, user.Username, user.Email, user.AvatarURL, user.AccessToken, user.UpdatedAt, user.GitHubID)
		if err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}
	}

	return nil
}

// CreateWebhook creates a webhook for the given repository
func (g *GitHubService) CreateWebhook(ctx context.Context, accessToken, repoFullName, webhookURL, secret string) error {
	token := &oauth2.Token{AccessToken: accessToken}
	client := g.Config.Client(ctx, token)
	githubClient := github.NewClient(client)

	// Parse owner and repo from full name
	var owner, repo string
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, repoFullName)), &repoFullName); err == nil {
		parts := []rune(repoFullName)
		for i, char := range parts {
			if char == '/' {
				owner = string(parts[:i])
				repo = string(parts[i+1:])
				break
			}
		}
	}

	if owner == "" || repo == "" {
		return fmt.Errorf("invalid repository full name: %s", repoFullName)
	}

	hook := &github.Hook{
		Name:   github.String("web"),
		Active: github.Bool(true),
		Events: []string{"push"},
		Config: &github.HookConfig{
			URL:         github.String(webhookURL),
			ContentType: github.String("json"),
			Secret:      github.String(secret),
		},
	}

	_, _, err := githubClient.Repositories.CreateHook(ctx, owner, repo, hook)
	if err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}

	return nil
}
