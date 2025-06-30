# Quick Setup Guide

Follow these steps to get GoTH Deployer running locally:

## 1. Prerequisites
- Go 1.23+ installed
- Git installed
- GitHub account

## 2. Clone and Install
```bash
git clone <this-repository>
cd deployer-golang-htmx
make install
```

## 3. GitHub OAuth Setup
1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Click "New OAuth App"
3. Fill in:
   - **Application name**: GoTH Deployer (Local)
   - **Homepage URL**: `http://localhost:8080`
   - **Authorization callback URL**: `http://localhost:8080/auth/github/callback`
4. Click "Register application"
5. Copy the **Client ID** and **Client Secret**

## 4. Environment Configuration
```bash
cp .env.example .env
```

Edit `.env` and add your GitHub OAuth credentials:
```env
GITHUB_CLIENT_ID=your_client_id_here
GITHUB_CLIENT_SECRET=your_client_secret_here
SESSION_SECRET=generate_a_random_string_here
```

## 5. Run the Application
```bash
make dev
```

## 6. Open Your Browser
Navigate to http://localhost:8080

## 7. Test Deployment
1. Click "Sign in with GitHub"
2. Click "New Project"
3. Select a GoTH stack repository
4. Click Deploy and watch the magic happen!

## Troubleshooting

### "No repositories available"
- Make sure your GitHub repositories are public or you have the correct permissions
- Ensure your OAuth app has the correct scopes (`repo`, `user:email`)

### Deployment fails
- Check that your repository has a `go.mod` file
- Ensure your main package is in `cmd/server/main.go`
- Verify your application accepts a `PORT` environment variable

### Server won't start
- Check that port 8080 is available
- Verify all environment variables are set correctly
- Run `make generate` to ensure templ files are compiled

## Next Steps
- Deploy your first GoTH application
- Explore the dashboard and deployment logs
- Check out the API endpoints for advanced usage

For more detailed information, see the main README.md file. 