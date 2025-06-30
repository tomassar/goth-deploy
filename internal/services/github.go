package services

import (
	"context"
	"fmt"

	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
	githubOAuth "golang.org/x/oauth2/github"
)

type GitHubService struct {
	clientID     string
	clientSecret string
	oauthConfig  *oauth2.Config
}

func NewGitHubService(clientID, clientSecret string) *GitHubService {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"repo", "user:email"},
		Endpoint:     githubOAuth.Endpoint,
	}

	return &GitHubService{
		clientID:     clientID,
		clientSecret: clientSecret,
		oauthConfig:  config,
	}
}

func (s *GitHubService) GetAuthURL(state string, redirectURL string) string {
	s.oauthConfig.RedirectURL = redirectURL
	return s.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (s *GitHubService) ExchangeCode(code string, redirectURL string) (*oauth2.Token, error) {
	s.oauthConfig.RedirectURL = redirectURL
	return s.oauthConfig.Exchange(context.Background(), code)
}

func (s *GitHubService) GetUser(token *oauth2.Token) (*github.User, error) {
	client := github.NewClient(s.oauthConfig.Client(context.Background(), token))
	user, _, err := client.Users.Get(context.Background(), "")
	return user, err
}

func (s *GitHubService) GetUserEmails(token *oauth2.Token) ([]*github.UserEmail, error) {
	client := github.NewClient(s.oauthConfig.Client(context.Background(), token))
	emails, _, err := client.Users.ListEmails(context.Background(), nil)
	return emails, err
}

func (s *GitHubService) ListRepositories(token *oauth2.Token) ([]*github.Repository, error) {
	client := github.NewClient(s.oauthConfig.Client(context.Background(), token))

	opt := &github.RepositoryListOptions{
		Visibility:  "all",
		Sort:        "updated",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []*github.Repository
	for {
		repos, resp, err := client.Repositories.List(context.Background(), "", opt)
		if err != nil {
			return nil, err
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allRepos, nil
}

func (s *GitHubService) GetRepository(token *oauth2.Token, owner, repo string) (*github.Repository, error) {
	client := github.NewClient(s.oauthConfig.Client(context.Background(), token))
	repository, _, err := client.Repositories.Get(context.Background(), owner, repo)
	return repository, err
}

func (s *GitHubService) GetLatestCommit(token *oauth2.Token, owner, repo, branch string) (*github.RepositoryCommit, error) {
	client := github.NewClient(s.oauthConfig.Client(context.Background(), token))

	opts := &github.CommitsListOptions{
		SHA:         branch,
		ListOptions: github.ListOptions{PerPage: 1},
	}

	commits, _, err := client.Repositories.ListCommits(context.Background(), owner, repo, opts)
	if err != nil {
		return nil, err
	}

	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found for branch %s", branch)
	}

	return commits[0], nil
}
