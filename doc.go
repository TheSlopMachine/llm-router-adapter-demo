// Package demo provides a reference implementation of a provider adapter.
//
// # Overview
//
// A provider adapter connects llm-router to an LLM backend. Each adapter consists of two parts:
//
//  1. Core Adapter: Implements provider.Adapter interface to handle API requests
//  2. Auth Flow Handler: Implements provider.AuthFlowHandler for automated credential acquisition (optional)
//
// The demo adapter serves as a complete reference implementation showing best practices
// for both components.
//
// # Quick Start: Creating a New Adapter
//
// Follow these steps to add a new provider:
//
//  1. Create a new package under providers/ (e.g., providers/openai/)
//  2. Implement the provider.Adapter interface (6 required methods)
//  3. Optionally implement provider.AuthFlowHandler for automated authentication
//  4. Register your adapter in init() by calling provider.Register(&YourAdapter{})
//  5. Import your package with a blank identifier in main.go
//
// Example minimal adapter:
//
//	package myapi
//
//	import "llm-router/internal/services/provider"
//
//	func init() {
//	    provider.Register(&Adapter{})
//	}
//
//	type Adapter struct{}
//
//	func (a *Adapter) TypeKey() string { return "myapi" }
//	func (a *Adapter) AuthType() models.AuthType { return models.AuthTypeAPIKey }
//	// ... implement remaining methods
//
// # Core Adapter Interface
//
// The provider.Adapter interface requires 6 methods:
//
// TypeKey() string
//   - Returns a unique identifier for this provider type
//   - Used in model IDs (e.g., "openai" for "openai/gpt-4")
//   - Must be lowercase, alphanumeric, no spaces
//
// AuthType() models.AuthType
//   - Declares what kind of credentials this provider uses
//   - Available types: AuthTypeAPIKey, AuthTypeOAuth2, AuthTypeBasic
//   - Used by the dashboard to display appropriate credential forms
//
// ValidateCredentials(data map[string]string) error
//   - Validates credential data before saving
//   - Check for required fields, format, etc.
//   - Return descriptive errors for user feedback
//
// Complete(ctx, cred, req) (*ChatCompletionResponse, error)
//   - Handles non-streaming chat completion requests
//   - Transform llm-router's request format to provider's API format
//   - Return standardized ChatCompletionResponse
//   - Return *provider.ProviderError for retryable errors (rate limits, quota)
//
// CompleteStream(ctx, cred, req, w) error
//   - Handles streaming chat completion requests
//   - Write Server-Sent Events (SSE) to the provided writer
//   - Format: "data: {json}\n\n" for each chunk
//   - Return *provider.ProviderError for retryable errors
//
// NeedsRefresh(cred) bool
//   - Returns true if credential needs refreshing (e.g., expired OAuth token)
//   - Called by maintenance service to check credential health
//
// RefreshCredential(ctx, cred) (*Credential, error)
//   - Obtains fresh credentials using existing credential data
//   - For OAuth: use refresh_token to get new access_token
//   - For static API keys: return provider.ErrNoRefreshNeeded
//
// # Authentication Flow Handler
//
// The provider.AuthFlowHandler interface enables automated credential acquisition
// through a wizard-like UI embedded in the dashboard. It uses a state machine model
// where the adapter controls the flow by returning state objects.
//
// ## State Machine Model
//
// The auth flow operates as a state machine:
//
//  1. User selects provider in wizard
//  2. Framework calls InitiateFlow() → adapter returns initial UI
//  3. User interacts with UI (submits form, clicks button)
//  4. Framework calls HandleStep() with user input → adapter returns next state
//  5. Repeat step 3-4 until adapter returns Credentials
//  6. Framework saves credentials and cleans up state
//
// ## AuthFlowContext
//
// The framework provides context to each auth flow method:
//
//	type AuthFlowContext struct {
//	    ProviderID string    // The provider instance ID
//	    FlowID     string    // Unique ID for this auth attempt
//	    Store      AuthStore // Key-value store for adapter state
//	}
//
// Use ctx.Store to persist data between steps:
//
//	// Save state
//	ctx.Store.Set(ctx.FlowID + ":username", username)
//
//	// Retrieve state
//	username, _ := ctx.Store.Get(ctx.FlowID + ":username")
//
// The framework automatically cleans up all state after credentials are saved.
//
// ## AuthFlowState
//
// Adapters return state objects to control the flow:
//
//	type AuthFlowState struct {
//	    RenderHTML    string            // HTML to display in wizard
//	    ExternalURL   string            // URL to open in new tab (OAuth)
//	    Credentials   map[string]string // Final credentials (flow complete)
//	    WaitingForCallback bool         // True if waiting for OAuth callback
//	}
//
// Exactly ONE of RenderHTML, ExternalURL, or Credentials should be set.
//
// # Authentication Patterns
//
// ## Pattern 1: Simple API Key Input
//
// The simplest pattern: show an input field, validate, return credentials.
//
//	func (f *APIKeyFlow) InitiateFlow(ctx provider.AuthFlowContext) (provider.AuthFlowState, error) {
//	    return provider.AuthFlowState{
//	        RenderHTML: `
//	            <div class="form-group">
//	                <label>API Key</label>
//	                <input name="api_key" type="password" placeholder="sk-..." required>
//	            </div>
//	            <button type="submit" class="btn btn-primary">Connect</button>`,
//	    }, nil
//	}
//
//	func (f *APIKeyFlow) HandleStep(ctx provider.AuthFlowContext, input map[string][]string) (provider.AuthFlowState, error) {
//	    apiKey := input["api_key"][0]
//
//	    // Validate the key (optional: make API call to verify)
//	    if !strings.HasPrefix(apiKey, "sk-") {
//	        return provider.AuthFlowState{
//	            RenderHTML: `<div class="error">Invalid API key format</div>
//	                         <input name="api_key" type="password" required>
//	                         <button type="submit" class="btn btn-primary">Retry</button>`,
//	        }, nil
//	    }
//
//	    // Flow complete
//	    return provider.AuthFlowState{
//	        Credentials: map[string]string{"api_key": apiKey},
//	    }, nil
//	}
//
// ## Pattern 2: OAuth2 Flow
//
// For providers that use OAuth2 (Google, GitHub, etc.):
//
//	func (o *OAuthFlow) InitiateFlow(ctx provider.AuthFlowContext) (provider.AuthFlowState, error) {
//	    return provider.AuthFlowState{
//	        RenderHTML: `
//	            <p>Connect your Google account to continue.</p>
//	            <button type="submit" class="btn btn-primary">Authorize with Google</button>`,
//	    }, nil
//	}
//
//	func (o *OAuthFlow) HandleStep(ctx provider.AuthFlowContext, input map[string][]string) (provider.AuthFlowState, error) {
//	    // Check if this is a callback (has 'code' parameter from OAuth provider)
//	    if code := input["code"]; len(code) > 0 {
//	        // Exchange authorization code for access token
//	        token, err := exchangeCodeForToken(code[0])
//	        if err != nil {
//	            return provider.AuthFlowState{
//	                RenderHTML: `<div class="error">Authentication failed: ` + err.Error() + `</div>`,
//	            }, nil
//	        }
//
//	        // Flow complete
//	        return provider.AuthFlowState{
//	            Credentials: map[string]string{
//	                "access_token":  token.AccessToken,
//	                "refresh_token": token.RefreshToken,
//	            },
//	        }, nil
//	    }
//
//	    // Initial submission: Generate OAuth URL
//	    pkceVerifier := generatePKCEVerifier()
//	    ctx.Store.Set(ctx.FlowID + ":pkce", pkceVerifier)
//
//	    oauthURL := "https://accounts.google.com/o/oauth2/auth?" +
//	        "client_id=YOUR_CLIENT_ID" +
//	        "&redirect_uri=http://localhost:8080/dashboard/providers/" + ctx.ProviderID + "/auth/callback" +
//	        "&state=" + ctx.FlowID +
//	        "&code_challenge=" + generateChallenge(pkceVerifier)
//
//	    return provider.AuthFlowState{
//	        ExternalURL:        oauthURL,
//	        WaitingForCallback: true,
//	        RenderHTML: `
//	            <p>Waiting for authorization...</p>
//	            <p>Please complete the authentication in the new tab.</p>`,
//	    }, nil
//	}
//
// ## Pattern 3: Multi-Step Flow (Username → Password → MFA)
//
// For providers requiring multiple steps:
//
//	func (m *MFAFlow) InitiateFlow(ctx provider.AuthFlowContext) (provider.AuthFlowState, error) {
//	    return provider.AuthFlowState{
//	        RenderHTML: `
//	            <div class="form-group">
//	                <label>Username</label>
//	                <input name="username" required>
//	            </div>
//	            <button type="submit" class="btn btn-primary">Next</button>`,
//	    }, nil
//	}
//
//	func (m *MFAFlow) HandleStep(ctx provider.AuthFlowContext, input map[string][]string) (provider.AuthFlowState, error) {
//	    // Determine current step
//	    step, _ := ctx.Store.Get(ctx.FlowID + ":step")
//
//	    switch step {
//	    case "": // Step 1: Username submitted
//	        username := input["username"][0]
//	        ctx.Store.Set(ctx.FlowID + ":step", "password")
//	        ctx.Store.Set(ctx.FlowID + ":username", username)
//
//	        return provider.AuthFlowState{
//	            RenderHTML: `
//	                <p>Welcome, ` + username + `</p>
//	                <div class="form-group">
//	                    <label>Password</label>
//	                    <input name="password" type="password" required>
//	                </div>
//	                <button type="submit" class="btn btn-primary">Next</button>`,
//	        }, nil
//
//	    case "password": // Step 2: Password submitted
//	        password := input["password"][0]
//	        // Validate credentials, trigger MFA...
//	        ctx.Store.Set(ctx.FlowID + ":step", "mfa")
//
//	        return provider.AuthFlowState{
//	            RenderHTML: `
//	                <p>Enter the code sent to your phone</p>
//	                <div class="form-group">
//	                    <input name="mfa_code" placeholder="000000" required>
//	                </div>
//	                <button type="submit" class="btn btn-primary">Verify</button>`,
//	        }, nil
//
//	    case "mfa": // Step 3: MFA code submitted
//	        mfaCode := input["mfa_code"][0]
//	        username, _ := ctx.Store.Get(ctx.FlowID + ":username")
//	        // Validate MFA code...
//
//	        return provider.AuthFlowState{
//	            Credentials: map[string]string{
//	                "username":      username,
//	                "session_token": "...",
//	            },
//	        }, nil
//	    }
//
//	    return provider.AuthFlowState{
//	        RenderHTML: `<div class="error">Invalid step</div>`,
//	    }, nil
//	}
//
// # UI Guidelines
//
// ## Available CSS Classes
//
// The framework provides these CSS classes for consistent styling:
//
//  - btn, btn-primary: Primary action button (blue)
//  - btn-secondary: Secondary action button (gray)
//  - btn-sm: Small button variant
//  - form-group: Container for form fields
//  - error: Error message styling (red)
//  - auth-flow-content: Recommended wrapper for your content
//
// ## Form Wrapping
//
// The framework automatically wraps your RenderHTML in a <form> tag with the correct
// action and method. You only need to provide the inner content.
//
// Your HTML:
//
//	<input name="api_key" required>
//	<button type="submit">Connect</button>
//
// Framework wraps it as:
//
//	<form method="POST" action="/dashboard/providers/{id}/auth">
//	    <input type="hidden" name="flow_id" value="...">
//	    <input name="api_key" required>
//	    <button type="submit">Connect</button>
//	</form>
//
// ## Error Handling
//
// For user-facing errors (invalid input, auth failure), return RenderHTML with an error message:
//
//	return provider.AuthFlowState{
//	    RenderHTML: `<div class="error">Invalid credentials. Please try again.</div>
//	                 <input name="api_key" type="password" required>
//	                 <button type="submit">Retry</button>`,
//	}, nil
//
// For system errors (network failure, parsing error), return an error:
//
//	return provider.AuthFlowState{}, fmt.Errorf("failed to contact API: %w", err)
//
// # Best Practices
//
// ## State Management
//
//  - Always prefix state keys with ctx.FlowID to avoid collisions
//  - Clean up sensitive data (passwords) from state after use
//  - Don't store credentials in state; return them in AuthFlowState.Credentials
//
// ## Security
//
//  - Use type="password" for sensitive input fields
//  - Validate all user input before processing
//  - Don't log or expose credentials in error messages
//  - For OAuth: Always use PKCE (Proof Key for Code Exchange)
//
// ## User Experience
//
//  - Provide clear, actionable error messages
//  - Show loading states for long operations
//  - Use descriptive button labels ("Connect with Google" not "Submit")
//  - Preserve user input when showing validation errors
//
// ## Testing
//
// Test your adapter locally:
//
//  1. Start llm-router: go run . localhost -p 8080
//  2. Navigate to http://localhost:8080/dashboard/credentials/new
//  3. Select your provider and test the auth flow
//  4. Verify credentials are saved correctly
//  5. Test API requests using the saved credentials
//
// # Common Pitfalls
//
//  - Don't construct framework URLs in your adapter (no coupling!)
//  - Don't call http.Redirect() - return ExternalURL instead
//  - Don't access ctx.Store directly for flow management - framework handles it
//  - Don't forget to handle OAuth callback parameters in HandleStep
//  - Don't return both RenderHTML and Credentials - choose one
//
// # Error Handling for Credential Rotation
//
// The framework implements intelligent retry with credential rotation. Adapters should
// return *provider.ProviderError for errors that should trigger credential rotation:
//
//	// Rate limit error (temporary, rotate to next credential)
//	if resp.StatusCode == 429 {
//	    retryAfter := time.Now().Add(60 * time.Second)
//	    return nil, &provider.ProviderError{
//	        StatusCode: 429,
//	        Message:    "rate limit exceeded",
//	        Type:       provider.ErrorTypeRateLimit,
//	        RetryAfter: &retryAfter,
//	    }
//	}
//
//	// Quota exceeded (credential exhausted, deprioritize)
//	if strings.Contains(body, "quota exceeded") {
//	    resetAt := time.Now().Add(24 * time.Hour)
//	    return nil, &provider.ProviderError{
//	        StatusCode: 429,
//	        Message:    "quota exceeded",
//	        Type:       provider.ErrorTypeQuotaExceeded,
//	        RetryAfter: &resetAt, // REQUIRED for ErrorTypeQuotaExceeded
//	    }
//	}
//
// Error types:
//  - ErrorTypeRateLimit: Temporary rate limit, rotate credential
//  - ErrorTypeQuotaExceeded: Credential quota exhausted, deprioritize (MUST have RetryAfter)
//  - ErrorTypeAuth: Auth failure, credential may be invalid
//  - ErrorTypeUpstream: Upstream error, don't retry
//  - ErrorTypeTimeout: Timeout, may retry
//
// The framework will:
//  1. Try each available credential once (sorted by LRU)
//  2. Apply exponential backoff when all credentials exhausted
//  3. Mark quota-exceeded credentials with lower priority
//  4. Automatically recover credentials when RetryAfter time passes
//
// # Additional Resources
//
// See the demo adapter implementation (adapter.go) for a complete working example.
// For the interface definitions, see internal/services/provider/adapter.go.
package demo
