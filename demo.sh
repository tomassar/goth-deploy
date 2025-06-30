#!/bin/bash

# GoTH Deployer Demo Script
# This script demonstrates the key features of the deployment platform

echo "🚀 GoTH Deployer Demo"
echo "===================="
echo ""

# Check if server is running
if ! curl -s http://localhost:8080 > /dev/null; then
    echo "❌ Server is not running on port 8080"
    echo "   Run: make dev"
    exit 1
fi

echo "✅ Server is running on http://localhost:8080"
echo ""

# Test API endpoints
echo "🔍 Testing API endpoints:"
echo ""

echo "📄 Home page (GET /):"
curl -s -o /dev/null -w "   Status: %{http_code}\n" http://localhost:8080/

echo "🔐 GitHub OAuth (GET /auth/github):"
curl -s -o /dev/null -w "   Status: %{http_code}\n" http://localhost:8080/auth/github

echo "📊 Dashboard (GET /dashboard) - redirects to home if not authenticated:"
curl -s -o /dev/null -w "   Status: %{http_code}\n" http://localhost:8080/dashboard

echo ""
echo "🎯 Key Features Demonstrated:"
echo "   ✨ Beautiful landing page with Tailwind CSS"
echo "   🔒 GitHub OAuth integration ready"
echo "   📱 Responsive design with HTMX"
echo "   🎨 Type-safe templates with Templ"
echo "   🏗️  Clean Go 1.23 architecture"
echo ""

echo "🌐 Open http://localhost:8080 in your browser to explore:"
echo "   • Modern, beautiful UI design"
echo "   • GitHub sign-in integration"
echo "   • Dashboard for project management"
echo "   • One-click deployment system"
echo ""

echo "📚 Next steps:"
echo "   1. Set up GitHub OAuth (see SETUP.md)"
echo "   2. Sign in with GitHub"
echo "   3. Deploy your first GoTH application!"
echo ""
echo "Happy deploying! 🎉" 