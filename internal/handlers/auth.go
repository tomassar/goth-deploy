package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
)

// GitHubAuthHandler initiates GitHub OAuth flow
func (h *Handler) GitHubAuthHandler(w http.ResponseWriter, r *http.Request) {
	// Generate random state for CSRF protection
	state, err := generateRandomState()
	if err != nil {
		log.Printf("Error generating state: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Store state in session
	session, err := h.Store.Get(r, "goth-session")
	if err != nil {
		log.Printf("Error getting session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	session.Values["oauth_state"] = state
	if err := session.Save(r, w); err != nil {
		log.Printf("Error saving session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Redirect to GitHub OAuth
	authURL := h.GitHub.GetAuthURL(state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// GitHubCallbackHandler handles GitHub OAuth callback
func (h *Handler) GitHubCallbackHandler(w http.ResponseWriter, r *http.Request) {
	// Get session
	session, err := h.Store.Get(r, "goth-session")
	if err != nil {
		log.Printf("Error getting session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Verify state parameter
	state := r.URL.Query().Get("state")
	storedState, ok := session.Values["oauth_state"].(string)
	if !ok || state != storedState {
		log.Printf("Invalid state parameter")
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		log.Printf("No authorization code received")
		http.Error(w, "No authorization code received", http.StatusBadRequest)
		return
	}

	// Exchange code for token
	token, err := h.GitHub.ExchangeCode(r.Context(), code)
	if err != nil {
		log.Printf("Error exchanging code for token: %v", err)
		http.Error(w, "Failed to exchange code for token", http.StatusInternalServerError)
		return
	}

	// Get user info from GitHub
	user, err := h.GitHub.GetUserInfo(r.Context(), token)
	if err != nil {
		log.Printf("Error getting user info: %v", err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	// Create or update user in database
	if err := h.GitHub.CreateOrUpdateUser(h.DB, user); err != nil {
		log.Printf("Error creating/updating user: %v", err)
		http.Error(w, "Failed to save user", http.StatusInternalServerError)
		return
	}

	// Store user ID in session
	session.Values["user_id"] = user.ID
	delete(session.Values, "oauth_state") // Clean up state
	if err := session.Save(r, w); err != nil {
		log.Printf("Error saving session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// LogoutHandler handles user logout
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// Get session
	session, err := h.Store.Get(r, "goth-session")
	if err != nil {
		// Even if there's an error, redirect to home
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Clear session
	session.Values = make(map[interface{}]interface{})
	session.Options.MaxAge = -1 // Delete the session

	if err := session.Save(r, w); err != nil {
		log.Printf("Error clearing session: %v", err)
	}

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// AuthMiddleware ensures the user is authenticated
func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := h.getCurrentUser(r)
		if user == nil {
			// Check if this is an HTMX request
			if r.Header.Get("HX-Request") == "true" {
				// For HTMX requests, redirect via HX-Redirect header
				w.Header().Set("HX-Redirect", "/")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			// For regular requests, redirect to home page
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// generateRandomState generates a random state string for OAuth
func generateRandomState() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
