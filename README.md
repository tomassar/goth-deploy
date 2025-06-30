# GoTH Deployer

A beautiful deployment platform specifically designed for the **GoTH stack** (Golang + HTMX + Tailwind + Templ). Deploy your server-side rendering applications with automatic subdomain creation, just like Netlify or Heroku, but optimized for Go applications using HTMX.

## Features

- 🚀 **One-Click Deployments** - Connect your GitHub repository and deploy instantly
- 🌐 **Automatic Subdomains** - Each project gets its own subdomain
- ⚡ **GoTH Stack Optimized** - Built-in support for templ generation and Go builds
- 📊 **Beautiful Dashboard** - Modern UI built with Tailwind CSS and HTMX
- 🔒 **GitHub Integration** - Secure OAuth authentication with GitHub
- 📝 **Real-time Logs** - Watch your deployments happen in real-time
- 🎯 **Zero Configuration** - Just connect and deploy

## Tech Stack

- **Backend**: Go 1.23 with idiomatic patterns
- **Frontend**: HTMX for dynamic interactions (minimal JavaScript)
- **Styling**: Tailwind CSS for beautiful, responsive design
- **Templates**: Templ for type-safe HTML templates
- **Database**: SQLite for simplicity
- **Authentication**: GitHub OAuth
- **Deployment**: Git-based with automatic builds

## Quick Start

### Prerequisites

- Go 1.23 or later
- Git
- GitHub account for OAuth app

### Installation

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd deployer-golang-htmx
   ```

2. **Install dependencies**
   ```bash
   go mod download
   go install github.com/a-h/templ/cmd/templ@latest
   ```

3. **Generate templ files**
   ```bash
   templ generate
   ```

4. **Set up GitHub OAuth**
   - Go to GitHub Settings > Developer settings > OAuth Apps
   - Create a new OAuth app with:
     - Homepage URL: `http://localhost:8080`
     - Authorization callback URL: `http://localhost:8080/auth/github/callback`
   - Copy the Client ID and Client Secret

5. **Configure environment variables**
   
   Create a `.env` file (or export directly):
   ```env
   GITHUB_CLIENT_ID=your_github_client_id
   GITHUB_CLIENT_SECRET=your_github_client_secret
   SESSION_SECRET=your_super_secret_session_key
   ```

6. **Run the application**
   ```bash
   go run cmd/server/main.go
   ```

7. **Open your browser**
   Navigate to `http://localhost:8080`

## Usage

### For End Users

1. **Sign in with GitHub** - Click the GitHub sign-in button
2. **Create a Project** - Click "New Project" and select a repository
3. **Deploy** - Click the deploy button and watch the magic happen
4. **Access Your App** - Your app will be available at `https://your-app.localhost:8080`

### For Developers

The platform automatically:
- Clones your repository
- Runs `go mod download` to install dependencies
- Generates templ files if they exist
- Builds your application with `go build`
- Starts your service with proper environment variables

## Project Structure

```
deployer-golang-htmx/
├── cmd/
│   └── server/           # Main application entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── database/        # Database connection and migrations
│   ├── handlers/        # HTTP handlers
│   ├── models/          # Data models
│   └── services/        # Business logic (GitHub, deployment)
├── web/
│   ├── templates/       # Templ template files
│   └── static/          # Static assets (CSS, JS, images)
├── data/                # SQLite database files
├── deployments/         # Deployed applications
└── README.md
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `8080` |
| `DATABASE_URL` | SQLite database path | `./data/app.db` |
| `GITHUB_CLIENT_ID` | GitHub OAuth client ID | Required |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth client secret | Required |
| `SESSION_SECRET` | Session encryption key | Required |
| `DEPLOYMENT_PATH` | Directory for deployments | `./deployments` |
| `BASE_DOMAIN` | Base domain for subdomains | `localhost:8080` |
| `ENVIRONMENT` | Application environment | `development` |

## Deployment Requirements

Your GoTH applications should:

1. **Have a `go.mod` file** in the root directory
2. **Main package** should be in `cmd/server/main.go`
3. **Use templ templates** (optional) - they'll be auto-generated
4. **Accept a `PORT` environment variable** for the server port
5. **Be buildable** with standard `go build` commands

Example main.go structure:
```go
package main

import (
    "net/http"
    "os"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    // Your HTMX + Templ application setup
    http.ListenAndServe(":"+port, handler)
}
```

## API Endpoints

- `GET /` - Landing page
- `GET /auth/github` - GitHub OAuth initiation
- `GET /auth/github/callback` - GitHub OAuth callback
- `GET /dashboard` - User dashboard
- `GET /projects` - List available repositories (HTMX)
- `POST /projects` - Create new project
- `GET /projects/{id}` - Project details
- `POST /projects/{id}/deploy` - Deploy project
- `GET /projects/{id}/logs` - Get deployment logs
- `DELETE /projects/{id}` - Delete project

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature-name`
3. Make your changes
4. Run tests: `go test ./...`
5. Generate templ files: `templ generate`
6. Commit your changes: `git commit -am 'Add feature'`
7. Push to the branch: `git push origin feature-name`
8. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Built with [templ](https://github.com/a-h/templ) for type-safe HTML templates
- Styled with [Tailwind CSS](https://tailwindcss.com/)
- Powered by [HTMX](https://htmx.org/) for dynamic interactions
- Uses [Chi](https://github.com/go-chi/chi) for HTTP routing