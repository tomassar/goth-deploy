# GoTH Deployer 🚀

A beautiful deployment platform for server-rendered web apps built with the **GoTH stack**:  
**Go 1.23 + HTMX + TailwindCSS + [templ](https://github.com/a-h/templ)**

## ⚡ What is it?

`goth-deploy` lets you connect a GitHub repository and deploy your GoTH app to a custom subdomain in seconds — like `myapp.example.com`.  
No YAML, no JS frameworks, no containers — just idiomatic Go and hypermedia.

## ✨ Features

- 🌀 **HTMX-native routing & interactivity** (zero custom JS)
- 🎨 **Templ-based server rendering** with type-safe templates
- 🌐 **Auto subdomain provisioning** per deployment
- 🔐 **GitHub OAuth integration** with seamless authentication
- 🚀 **One-click deployments** with real-time build logs
- ⚙️ **Environment variable management** via beautiful UI
- 📊 **Beautiful dashboard** with deployment stats and project overview
- 🧰 **Minimal config** (convention over configuration)
- 🔒 **Secure and idiomatic Go 1.23 backend**

## 🏗️ Architecture

The application follows a clean architecture pattern:

```
goth-deploy/
├── cmd/server/              # Application entry point
├── internal/
│   ├── config/             # Configuration management
│   ├── database/           # SQLite database and migrations
│   ├── handlers/           # HTTP handlers and routing
│   ├── models/            # Data models and types
│   └── services/          # Business logic (GitHub, deployment, proxy)
└── web/templates/         # Templ templates with Tailwind CSS
```

## 🚀 Quick Start

### Prerequisites

- Go 1.23+
- Git
- GitHub account

### 1. Clone and Setup

```bash
git clone <repository-url>
cd goth-deploy
make setup
```

### 2. Configure GitHub OAuth

1. Go to [GitHub Developer Settings](https://github.com/settings/applications/new)
2. Create a new OAuth App with:
   - **Application name**: GoTH Deployer
   - **Homepage URL**: `http://localhost:8080`
   - **Authorization callback URL**: `http://localhost:8080/auth/github/callback`
3. Copy your Client ID and Client Secret

### 3. Environment Configuration

Create a `.env` file:

```bash
# Server Configuration
PORT=8080
DATABASE_URL=./data/app.db
SESSION_SECRET=your-super-secret-session-key-change-this-in-production

# GitHub OAuth Configuration
GITHUB_CLIENT_ID=your-github-client-id
GITHUB_CLIENT_SECRET=your-github-client-secret
GITHUB_REDIRECT_URL=http://localhost:8080/auth/github/callback

# Deployment Configuration
DEPLOYMENT_ROOT=./deployments
BASE_DOMAIN=localhost:8080
ENABLE_HTTPS=false

# Optional: GitHub Webhook Secret for automatic deployments
GITHUB_WEBHOOK_SECRET=your-webhook-secret
```

### 4. Run the Application

```bash
make run
```

Visit [http://localhost:8080](http://localhost:8080) and sign in with GitHub!

## 🛠️ Development

### Available Commands

```bash
make build      # Build the application
make run        # Build and run
make dev        # Development mode with hot reload (requires air)
make templ      # Generate templ files
make deps       # Install dependencies
make clean      # Clean build artifacts
make test       # Run tests
make fmt        # Format code
```

### Project Structure

- **Templates**: `web/templates/` - Templ templates with Tailwind CSS
- **Handlers**: `internal/handlers/` - HTTP request handlers
- **Services**: `internal/services/` - Business logic
- **Models**: `internal/models/` - Data structures
- **Database**: `internal/database/` - SQLite with migrations

### Tech Stack Details

- **Backend**: Go 1.23 with Chi router
- **Frontend**: HTMX + TailwindCSS (via CDN)
- **Templates**: Templ for type-safe HTML generation
- **Database**: SQLite with migrations
- **Authentication**: GitHub OAuth 2.0
- **Sessions**: Secure cookie-based sessions

## 🌟 How It Works

1. **Authentication**: Users sign in with GitHub OAuth
2. **Repository Selection**: Browse and select Go repositories
3. **Project Creation**: Configure build/start commands and subdomain
4. **Deployment**: One-click deployment with real-time logs
5. **Proxy**: Automatic reverse proxy to serve apps on subdomains
6. **Management**: Environment variables, build logs, and project settings

## 🎯 Deployment Flow

1. User clicks "Deploy" on a project
2. System clones the GitHub repository
3. Runs the build command in the project directory
4. Starts the application with the start command
5. Sets up reverse proxy for the subdomain
6. Updates project status and deployment logs

## 📋 Supported Project Types

Currently optimized for Go applications, particularly those using:
- Go HTTP servers
- GoTH stack applications (Go + HTMX + Tailwind + Templ)
- Any Go application that can be built with `go build`

## 🔧 Configuration

### Default Build/Start Commands

- **Build Command**: `go build -o main .`
- **Start Command**: `./main`
- **Port**: `8080` (configurable per project)

### Environment Variables

Projects can have custom environment variables managed through the UI:
- Secure storage in database
- Available during build and runtime
- Easy management via HTMX interface

## 🚀 Production Deployment

For production deployment:

1. Use a proper domain with wildcard DNS (`*.yourdomain.com`)
2. Set up SSL/TLS certificates
3. Configure proper session secrets
4. Use a more robust database (PostgreSQL recommended)
5. Set up GitHub webhooks for automatic deployments
6. Configure firewall and security settings

## 🤝 Contributing

This is an experimental project aimed at the Go community building beautiful, dynamic apps with a minimalist toolchain.

## 📄 License

MIT License - see LICENSE file for details.

---

**Made with 💀 by GoTH stack fans.**

*Deploy your Go + HTMX + Tailwind applications with style!*