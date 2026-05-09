// Package demo provides a stub Provider Adapter that returns fixed demo responses.
//
// This adapter demonstrates how to add a new provider to llm-router.
// It serves as a complete reference implementation showing both the core adapter
// interface and the optional authentication flow handler.
//
// To use this adapter as a template:
//  1. Copy this package to providers/yourprovider/
//  2. Update TypeKey() to return your provider's identifier
//  3. Implement the API request methods (Complete, CompleteStream)
//  4. Optionally implement GetAuthFlow() for automated credential acquisition
//  5. Register by importing with blank identifier in main.go
//
// For comprehensive documentation, see doc.go in this package.
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	sdk "github.com/TheSlopMachine/llm-router-sdk"
)

func init() {
	// Register this adapter with the framework.
	// The framework will automatically discover and use this adapter
	// for any provider configured with type="demo".
	sdk.Register(&Adapter{
		rateLimiters: make(map[string]*rateLimiter),
	})
}

// Adapter implements provider.Adapter for the demo stub provider.
// This is a stateless struct - all per-provider state lives in Credential records.
type Adapter struct {
	mu            sync.Mutex
	rateLimiters  map[string]*rateLimiter // credID -> rate limiter
	
	// TestConfig enables test-specific behaviors (zero value = production)
	TestConfig    TestConfig
}

// TestConfig configures adapter behavior for testing.
type TestConfig struct {
	// DisableRateLimit disables rate limiting for fast tests
	DisableRateLimit bool
	
	// DisableQuota disables quota limiting
	DisableQuota bool
	
	// ResponseFunc overrides default response content
	ResponseFunc func(req *sdk.ChatCompletionRequest) string
	
	// ForceError forces this error to be returned (overrides all other logic)
	ForceError *sdk.ProviderError
	
	// ResponseDelay adds delay before responding (for timeout/cancellation tests)
	ResponseDelay time.Duration
	
	// ModelBehaviors defines model-specific behaviors (keyed by model name without prefix)
	ModelBehaviors map[string]ModelBehavior
}

// ModelBehavior defines behavior for a specific model.
type ModelBehavior struct {
	Error       *sdk.ProviderError  // Return this error
	Response    string                    // Return this content
	StreamDelay time.Duration            // Delay between stream chunks
}

// rateLimiter tracks request timestamps for rate limiting simulation.
type rateLimiter struct {
	requests     []time.Time
	requestCount int64
}

const (
	rateLimit = 10                  // 10 requests per minute
	quotaLimit = 50                 // 50 requests total before quota exceeded
)

// TypeKey returns the unique identifier for this provider type.
//
// This value is used in two places:
//  1. Provider.Type field in the database
//  2. Model ID prefix (e.g., "demo/hello-model")
//
// Requirements:
//  - Must be lowercase
//  - Must be alphanumeric (no spaces or special characters)
//  - Must be unique across all registered adapters
func (a *Adapter) TypeKey() string { return "demo" }

// AuthType declares what kind of credentials this provider uses.
//
// Available types:
//  - sdk.AuthTypeAPIKey: Simple API key (most common)
//  - sdk.AuthTypeOAuth2: OAuth2 access/refresh tokens
//  - sdk.AuthTypeBasic: HTTP Basic Auth (username/password)
//
// This is used by the dashboard to determine how to display credential forms
// and by the framework to validate credential structure.
func (a *Adapter) AuthType() sdk.AuthType { return sdk.AuthTypeAPIKey }

// ValidateCredentials checks that the credential data is valid before saving.
//
// This method is called when:
//  - A user manually adds a credential via the dashboard
//  - An auth flow completes and returns credential data
//
// Best practices:
//  - Check for required fields
//  - Validate field formats (e.g., API key prefix)
//  - Optionally make a test API call to verify the credential works
//  - Return descriptive errors that help users fix the issue
//
// Do NOT:
//  - Modify the data parameter (it's read-only)
//  - Store credentials here (framework handles storage)
//  - Make expensive API calls (this is called synchronously)
func (a *Adapter) ValidateCredentials(data map[string]string) error {
	if data["demo_key"] == "" {
		return fmt.Errorf("demo: credential must include a non-empty \"demo_key\"")
	}
	return nil
}

const demoMessage = "Welcome to llm-router! This is a demo provider that always returns this fixed message. " +
	"Adding a new provider is simple: create a package under providers/, implement the provider.Adapter interface " +
	"with six methods (TypeKey, AuthType, ValidateCredentials, Complete, CompleteStream, NeedsRefresh, and RefreshCredential), " +
	"then register it in your init() function by calling provider.Register(). " +
	"That's all you need to route requests to any LLM backend."

// Complete handles non-streaming chat completion requests.
//
// This method is called when a client makes a POST to /v1/chat/completions
// without the "stream": true parameter.
//
// Implementation steps:
//  1. Extract credentials from cred.Data (e.g., API key)
//  2. Transform req (llm-router format) to your provider's API format
//  3. Make HTTP request to provider's API
//  4. Parse response and transform to sdk.ChatCompletionResponse
//  5. Return the standardized response
//
// Error handling:
//  - Return errors for network failures, API errors, parsing errors
//  - Include provider error messages in returned errors for debugging
//  - Check ctx.Done() for client cancellation on long requests
//
// The demo implementation simulates rate limiting (10 req/min) and quota limits (50 total).
func (a *Adapter) Complete(
	ctx context.Context,
	cred *sdk.Credential,
	req *sdk.ChatCompletionRequest,
) (*sdk.ChatCompletionResponse, error) {
	// Check forced error
	if a.TestConfig.ForceError != nil {
		return nil, a.TestConfig.ForceError
	}
	
	// Check response delay
	if a.TestConfig.ResponseDelay > 0 {
		select {
		case <-time.After(a.TestConfig.ResponseDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	
	// Extract model name without prefix
	modelName := extractModelName(req.Model)
	
	// Check model-specific behavior
	if behavior, ok := a.TestConfig.ModelBehaviors[modelName]; ok {
		if behavior.Error != nil {
			return nil, behavior.Error
		}
		if behavior.Response != "" {
			return &sdk.ChatCompletionResponse{
				ID:      "demo-" + fmt.Sprintf("%d", time.Now().Unix()),
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   string(req.Model),
				Choices: []sdk.ChatCompletionChoice{
					{
						Index: 0,
						Message: sdk.ChatMessage{
							Role:    "assistant",
							Content: behavior.Response,
						},
						FinishReason: "stop",
					},
				},
				Usage: sdk.ChatCompletionUsage{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
				},
			}, nil
		}
	}
	
	// Handle built-in test models
	switch modelName {
	case "rate-limit-model":
		resetAt := time.Now().Add(60 * time.Second)
		return nil, &sdk.ProviderError{
			StatusCode: 429,
			Message:    "rate limit exceeded for test model",
			Type:       sdk.ErrorTypeRateLimit,
			RetryAfter: &resetAt,
		}
	case "quota-model":
		resetAt := time.Now().Add(24 * time.Hour)
		return nil, &sdk.ProviderError{
			StatusCode: 429,
			Message:    "quota exceeded for test model",
			Type:       sdk.ErrorTypeQuotaExceeded,
			RetryAfter: &resetAt,
		}
	case "auth-error-model":
		return nil, &sdk.ProviderError{
			StatusCode: 401,
			Message:    "authentication failed for test model",
			Type:       sdk.ErrorTypeAuth,
		}
	case "upstream-error-model":
		return nil, &sdk.ProviderError{
			StatusCode: 500,
			Message:    "upstream error for test model",
			Type:       sdk.ErrorTypeUpstream,
		}
	case "timeout-model":
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case "network-error-model":
		return nil, fmt.Errorf("network error: connection refused")
	case "malformed-model":
		return nil, fmt.Errorf("malformed response from provider")
	}
	
	// Check rate limit (unless disabled)
	if !a.TestConfig.DisableRateLimit && !a.TestConfig.DisableQuota {
		if err := a.checkRateLimit(cred.ID); err != nil {
			return nil, err
		}
	}
	
	// Generate response content
	responseContent := demoMessage
	if a.TestConfig.ResponseFunc != nil {
		responseContent = a.TestConfig.ResponseFunc(req)
	}

	return &sdk.ChatCompletionResponse{
		ID:      "demo-" + fmt.Sprintf("%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   string(req.Model),
		Choices: []sdk.ChatCompletionChoice{
			{
				Index: 0,
				Message: sdk.ChatMessage{
					Role:    "assistant",
					Content: responseContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: sdk.ChatCompletionUsage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}, nil
}

// CompleteStream handles streaming chat completion requests.
//
// This method is called when a client makes a POST to /v1/chat/completions
// with "stream": true.
//
// Implementation steps:
//  1. Extract credentials from cred.Data
//  2. Transform req to your provider's API format
//  3. Make streaming HTTP request to provider's API
//  4. Read response stream and transform each chunk to sdk.StreamChunk
//  5. Write each chunk as Server-Sent Event (SSE) to w
//
// SSE format:
//  - Each event: "data: {json}\n\n"
//  - Final event: "data: [DONE]\n\n" (optional, depends on provider)
//
// Error handling:
//  - Check ctx.Done() frequently to detect client disconnection
//  - Return errors for network/parsing failures
//  - Partial writes are acceptable (client may have disconnected)
//
// The demo implementation simulates rate limiting and streams the response word-by-word.
func (a *Adapter) CompleteStream(
	ctx context.Context,
	cred *sdk.Credential,
	req *sdk.ChatCompletionRequest,
	w io.Writer,
) error {
	// Check forced error
	if a.TestConfig.ForceError != nil {
		return a.TestConfig.ForceError
	}
	
	// Check response delay
	if a.TestConfig.ResponseDelay > 0 {
		select {
		case <-time.After(a.TestConfig.ResponseDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	// Extract model name without prefix
	modelName := extractModelName(req.Model)
	
	// Check model-specific behavior
	if behavior, ok := a.TestConfig.ModelBehaviors[modelName]; ok {
		if behavior.Error != nil {
			return behavior.Error
		}
	}
	
	// Handle built-in test models
	switch modelName {
	case "rate-limit-model":
		resetAt := time.Now().Add(60 * time.Second)
		return &sdk.ProviderError{
			StatusCode: 429,
			Message:    "rate limit exceeded for test model",
			Type:       sdk.ErrorTypeRateLimit,
			RetryAfter: &resetAt,
		}
	case "quota-model":
		resetAt := time.Now().Add(24 * time.Hour)
		return &sdk.ProviderError{
			StatusCode: 429,
			Message:    "quota exceeded for test model",
			Type:       sdk.ErrorTypeQuotaExceeded,
			RetryAfter: &resetAt,
		}
	case "auth-error-model":
		return &sdk.ProviderError{
			StatusCode: 401,
			Message:    "authentication failed for test model",
			Type:       sdk.ErrorTypeAuth,
		}
	case "upstream-error-model":
		return &sdk.ProviderError{
			StatusCode: 500,
			Message:    "upstream error for test model",
			Type:       sdk.ErrorTypeUpstream,
		}
	case "timeout-model":
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	case "network-error-model":
		return fmt.Errorf("network error: connection refused")
	case "malformed-model":
		return fmt.Errorf("malformed response from provider")
	}
	
	// Check rate limit (unless disabled)
	if !a.TestConfig.DisableRateLimit && !a.TestConfig.DisableQuota {
		if err := a.checkRateLimit(cred.ID); err != nil {
			return err
		}
	}

	id := "demo-" + fmt.Sprintf("%d", time.Now().Unix())
	created := time.Now().Unix()

	// Generate response content
	content := demoMessage
	if a.TestConfig.ResponseFunc != nil {
		content = a.TestConfig.ResponseFunc(req)
	}
	
	// Extract model name without prefix
	streamModelName := extractModelName(req.Model)
	
	// Check for model-specific response
	if behavior, ok := a.TestConfig.ModelBehaviors[streamModelName]; ok {
		if behavior.Response != "" {
			content = behavior.Response
		}
	}

	// Split message into words for streaming simulation
	words := splitWords(content)
	
	// Determine stream delay
	streamDelay := 100 * time.Millisecond
	if bhv, ok := a.TestConfig.ModelBehaviors[streamModelName]; ok && bhv.StreamDelay > 0 {
		streamDelay = bhv.StreamDelay
	}

	// Stream each word as a separate chunk
	for i, word := range words {
		// Check for client cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Add space after each word except the last
		wordContent := word
		if i < len(words)-1 {
			wordContent += " "
		}

		// Create chunk with delta content
		chunk := sdk.StreamChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   string(req.Model),
			Choices: []sdk.StreamChunkChoice{
				{
					Index: 0,
					Delta: sdk.ChatMessage{
						Role:    "assistant",
						Content: wordContent,
					},
					FinishReason: nil,
				},
			},
		}

		// Marshal and write as SSE
		data, err := json.Marshal(chunk)
		if err != nil {
			return fmt.Errorf("demo: marshal chunk: %w", err)
		}

		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}

		// Artificial delay to simulate streaming
		time.Sleep(streamDelay)
	}

	// Send final chunk with finish_reason
	finishReason := "stop"
	finalChunk := sdk.StreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   string(req.Model),
		Choices: []sdk.StreamChunkChoice{
			{
				Index: 0,
				Delta: sdk.ChatMessage{
					Role:    "assistant",
					Content: "",
				},
				FinishReason: &finishReason,
			},
		},
	}

	data, err := json.Marshal(finalChunk)
	if err != nil {
		return fmt.Errorf("demo: marshal final chunk: %w", err)
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}

	return nil
}

// NeedsRefresh returns true if the credential needs to be refreshed.
//
// This method is called periodically by the maintenance service to check
// credential health. Return true if:
//  - OAuth token is expired or will expire soon
//  - Session token needs renewal
//  - Any other condition that requires credential refresh
//
// For static API keys that don't expire, always return false.
//
// The maintenance service will call RefreshCredential() if this returns true.
func (a *Adapter) NeedsRefresh(cred *sdk.Credential) bool {
	// Demo credentials never expire
	return false
}

// RefreshCredential obtains fresh credentials using the existing credential data.
//
// This method is called by the maintenance service when NeedsRefresh() returns true.
//
// Implementation patterns:
//
// For OAuth2:
//  1. Extract refresh_token from cred.Data
//  2. Make token refresh request to provider's OAuth endpoint
//  3. Parse response and create new Credential with updated tokens
//  4. Return the new credential (framework will save it)
//
// For session-based auth:
//  1. Extract session credentials from cred.Data
//  2. Make session renewal request
//  3. Return updated credential with new session token
//
// For static API keys:
//  - Return sdk.ErrNoRefreshNeeded
//
// Error handling:
//  - Return errors for network failures, invalid refresh tokens, etc.
//  - The framework will retry later on transient errors
//  - On permanent errors (invalid refresh token), the credential will be marked as failed
func (a *Adapter) RefreshCredential(_ context.Context, _ *sdk.Credential) (*sdk.Credential, error) {
	// Demo credentials don't need refreshing
	return nil, sdk.ErrNoRefreshNeeded
}

// ─────────────────────────────────────────────
// Authentication Flow Handler
// ─────────────────────────────────────────────

// DemoAuthFlow implements sdk.AuthFlowHandler for the demo provider.
//
// This demonstrates the simplest possible auth flow: a single-step confirmation.
// The user clicks a button, and credentials are immediately generated.
//
// For more complex patterns (OAuth, multi-step, etc.), see doc.go in this package.
type DemoAuthFlow struct{}

// InitiateFlow returns the initial authentication UI.
//
// This method is called when the user selects this provider in the credential wizard.
// The returned HTML will be embedded in the wizard's Step 2 and automatically wrapped
// in a <form> by the framework.
//
// Best practices:
//  - Keep the UI simple and focused
//  - Use framework CSS classes (btn, btn-primary, form-group)
//  - Provide clear instructions
//  - Don't include <form> tags (framework adds them)
//
// The demo implementation shows a simple confirmation button.
func (f *DemoAuthFlow) InitiateFlow(ctx sdk.AuthFlowContext) (sdk.AuthFlowState, error) {
	return sdk.AuthFlowState{
		RenderHTML: `
<div class="auth-flow-content">
	<p><strong>Demo Authentication</strong></p>
	<p>This is a mock authentication for the demo provider. Click below to confirm.</p>
	<button type="submit" class="btn btn-primary">Confirm Authentication</button>
</div>`,
	}, nil
}

// HandleStep processes user input and returns the next state.
//
// This method is called when:
//  - The user submits a form in the wizard
//  - An OAuth provider redirects back with a callback
//  - Any other interaction that triggers a POST
//
// Parameters:
//  - ctx: Provides ProviderID, FlowID, and Store for state management
//  - input: Form data or query parameters as map[string][]string
//
// Return values:
//  - RenderHTML: Show next step UI (or error message)
//  - ExternalURL: Redirect to external OAuth site
//  - Credentials: Flow complete, save these credentials
//
// State management:
//  - Use ctx.Store.Set(ctx.FlowID + ":key", value) to persist data between steps
//  - Use ctx.Store.Get(ctx.FlowID + ":key") to retrieve saved data
//  - Framework automatically cleans up state after credentials are saved
//
// Error handling:
//  - Return errors for system failures (network, parsing, etc.)
//  - For user errors (invalid input), return RenderHTML with error message
//
// The demo implementation immediately returns credentials without validation.
func (f *DemoAuthFlow) HandleStep(ctx sdk.AuthFlowContext, input map[string][]string) (sdk.AuthFlowState, error) {
	// Demo provider: any submission completes the flow
	// In a real implementation, you would:
	//  1. Validate input data
	//  2. Make API calls to verify credentials
	//  3. Handle multi-step flows using ctx.Store
	//  4. Return appropriate state based on validation results
	
	return sdk.AuthFlowState{
		Credentials: map[string]string{
			"demo_key": "demo-secret-key-" + fmt.Sprintf("%d", time.Now().Unix()),
		},
	}, nil
}

// GetAuthFlow returns the auth flow handler for this adapter.
//
// Return nil if your provider doesn't support automated authentication
// (users will need to manually enter credentials via JSON).
//
// Return an AuthFlowHandler implementation to enable the wizard-based
// credential acquisition flow in the dashboard.
func (a *Adapter) GetAuthFlow() sdk.AuthFlowHandler {
	return &DemoAuthFlow{}
}

// GetDefaultProviders returns the list of default providers for this adapter.
// Demo provider should not be added automatically as it's not a real LLM provider.
func (a *Adapter) GetDefaultProviders() []sdk.ProviderInfo {
	return []sdk.ProviderInfo{
		{
			Name:      "Demo Provider",
			Qualifier: "",
			BaseURL:   "",
			IconURL:   "data:image/svg+xml,%3Csvg width='32' height='32' xmlns='http://www.w3.org/2000/svg'%3E%3Crect width='32' height='32' fill='%236c717a'/%3E%3Ctext x='50%25' y='50%25' font-family='Arial' font-size='18' fill='%23ffffff' text-anchor='middle' dy='.3em'%3ED%3C/text%3E%3C/svg%3E",
		},
	}
}

// GetModelInfos returns metadata for all available models from the demo provider.
// Demo implementation returns hardcoded model information.
func (a *Adapter) GetModelInfos(ctx context.Context, cred *sdk.Credential, providerQualifier string) ([]sdk.ModelInfo, error) {
	return []sdk.ModelInfo{
		{
			Name:          "hello-model",
			DisplayName:   "Demo Hello Model",
			RPM:           100,
			TPM:           10000,
			RPD:           10000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
		{
			Name:          "echo-model",
			DisplayName:   "Demo Echo Model",
			RPM:           200,
			TPM:           20000,
			RPD:           20000,
			ContextWindow: 8192,
			MaxTokens:     4096,
		},
		{
			Name:          "success-model",
			DisplayName:   "Always Succeeds",
			RPM:           1000,
			TPM:           100000,
			RPD:           100000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
		{
			Name:          "rate-limit-model",
			DisplayName:   "Always Rate Limited",
			RPM:           10,
			TPM:           1000,
			RPD:           1000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
		{
			Name:          "quota-model",
			DisplayName:   "Always Quota Exceeded",
			RPM:           100,
			TPM:           10000,
			RPD:           10000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
		{
			Name:          "auth-error-model",
			DisplayName:   "Always Auth Error",
			RPM:           100,
			TPM:           10000,
			RPD:           10000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
		{
			Name:          "upstream-error-model",
			DisplayName:   "Always Upstream Error",
			RPM:           100,
			TPM:           10000,
			RPD:           10000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
		{
			Name:          "timeout-model",
			DisplayName:   "Delays 10 Seconds",
			RPM:           100,
			TPM:           10000,
			RPD:           10000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
		{
			Name:          "network-error-model",
			DisplayName:   "Network Failure",
			RPM:           100,
			TPM:           10000,
			RPD:           10000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
		{
			Name:          "malformed-model",
			DisplayName:   "Invalid Response",
			RPM:           100,
			TPM:           10000,
			RPD:           10000,
			ContextWindow: 4096,
			MaxTokens:     2048,
		},
	}, nil
}

// ─────────────────────────────────────────────
// Helper Functions
// ─────────────────────────────────────────────

// checkRateLimit simulates rate limiting for testing credential rotation.
// Returns ProviderError with appropriate type and RetryAfter.
func (a *Adapter) checkRateLimit(credID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Get or create rate limiter for this credential
	rl, exists := a.rateLimiters[credID]
	if !exists {
		rl = &rateLimiter{
			requests: make([]time.Time, 0),
		}
		a.rateLimiters[credID] = rl
	}

	now := time.Now()

	// Remove requests older than 1 minute
	cutoff := now.Add(-1 * time.Minute)
	validRequests := make([]time.Time, 0)
	for _, t := range rl.requests {
		if t.After(cutoff) {
			validRequests = append(validRequests, t)
		}
	}
	rl.requests = validRequests

	// Check quota limit (50 total requests)
	if rl.requestCount >= quotaLimit {
		resetAt := now.Add(24 * time.Hour)
		return &sdk.ProviderError{
			StatusCode: 429,
			Message:    fmt.Sprintf("quota exceeded: %d requests used, limit is %d", rl.requestCount, quotaLimit),
			Type:       sdk.ErrorTypeQuotaExceeded,
			RetryAfter: &resetAt,
		}
	}

	// Check rate limit (10 req/min)
	if len(rl.requests) >= rateLimit {
		resetAt := now.Add(60 * time.Second)
		return &sdk.ProviderError{
			StatusCode: 429,
			Message:    fmt.Sprintf("rate limit exceeded: %d requests in last minute, limit is %d/min", len(rl.requests), rateLimit),
			Type:       sdk.ErrorTypeRateLimit,
			RetryAfter: &resetAt,
		}
	}

	// Record this request
	rl.requests = append(rl.requests, now)
	rl.requestCount++

	return nil
}

// splitWords splits a string into words, preserving punctuation with the preceding word.
// Used by CompleteStream to simulate word-by-word streaming.
func splitWords(s string) []string {
	var words []string
	var current []rune

	for _, r := range s {
		if r == ' ' {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
		} else {
			current = append(current, r)
		}
	}

	if len(current) > 0 {
		words = append(words, string(current))
	}

	return words
}

// extractModelName extracts the model name without the provider prefix.
// For example, "demo/hello-model" returns "hello-model".
func extractModelName(modelID sdk.ModelId) string {
	s := string(modelID)
	if idx := len(s) - 1; idx >= 0 {
		for i := len(s) - 1; i >= 0; i-- {
			if s[i] == '/' {
				return s[i+1:]
			}
		}
	}
	return s
}

// NewTestAdapter creates a demo adapter configured for testing.
func NewTestAdapter(config TestConfig) *Adapter {
	return &Adapter{
		rateLimiters: make(map[string]*rateLimiter),
		TestConfig:   config,
	}
}

// ─────────────────────────────────────────────
// Compile-time Interface Verification
// ─────────────────────────────────────────────

// Verify that Adapter implements sdk.Adapter at compile time.
// If this line causes a compilation error, it means the Adapter struct
// is missing required methods from the sdk.Adapter interface.
var _ sdk.Adapter = (*Adapter)(nil)

