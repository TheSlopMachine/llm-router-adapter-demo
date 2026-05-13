# llm-router-adapter-demo

Reference implementation demonstrating the llm-router adapter interface.

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

---

## Overview

The **llm-router-adapter-demo** is a complete reference implementation of the [llm-router-sdk](https://github.com/TheSlopMachine/llm-router-sdk) adapter interface. It serves as a template and learning resource for building your own provider adapters.

This adapter returns fixed demo responses and simulates various provider behaviors (rate limiting, quota exhaustion, errors) to demonstrate proper error handling and credential rotation patterns.

---

## Purpose

Use this adapter as a reference when building your own provider adapters. It demonstrates:

- ✅ **Complete Adapter implementation** - All 10 interface methods
- ✅ **Auth flow handler** - Simple confirmation-based authentication
- ✅ **Rate limiting simulation** - 10 requests per minute
- ✅ **Quota limiting simulation** - 50 total requests before quota exceeded
- ✅ **Error handling patterns** - Proper `ProviderError` usage for credential rotation
- ✅ **Streaming support** - Word-by-word SSE streaming
- ✅ **Model metadata** - `GetModelInfos()` implementation
- ✅ **Test models** - Built-in models for testing error scenarios
- ✅ **Comprehensive documentation** - Inline comments and doc.go guide

---

## Features

### Core Adapter

Implements all required methods from `sdk.Adapter`:
- `TypeKey()` - Returns `"demo"`
- `AuthType()` - Uses `AuthTypeAPIKey`
- `ValidateCredentials()` - Validates `demo_key` field
- `Complete()` - Returns fixed demo message
- `CompleteStream()` - Streams response word-by-word
- `NeedsRefresh()` - Always returns false (static credentials)
- `RefreshCredential()` - Returns `ErrNoRefreshNeeded`
- `GetModelInfos()` - Returns hardcoded model metadata
- `GetAuthFlow()` - Returns simple confirmation flow
- `GetDefaultProviders()` - Returns demo provider info

### Test Models

Built-in models for testing various scenarios:

| Model | Behavior |
|-------|----------|
| `demo/hello-model` | Normal response with demo message |
| `demo/echo-model` | Normal response (alias) |
| `demo/success-model` | Always succeeds |
| `demo/rate-limit-model` | Always returns 429 rate limit error |
| `demo/quota-model` | Always returns quota exceeded error |
| `demo/auth-error-model` | Always returns 401 authentication error |
| `demo/upstream-error-model` | Always returns 500 upstream error |
| `demo/timeout-model` | Delays 10 seconds before responding |
| `demo/network-error-model` | Simulates network connection failure |
| `demo/malformed-model` | Simulates malformed response parsing error |

### Rate Limiting

Simulates realistic rate limiting behavior:
- **Rate limit**: 10 requests per minute per credential
- **Quota limit**: 50 total requests per credential
- Returns `ProviderError` with `ErrorTypeRateLimit` or `ErrorTypeQuotaExceeded`
- Includes `RetryAfter` timestamp for automatic recovery

### Authentication Flow

Demonstrates the simplest possible auth flow:
1. User clicks "Confirm Authentication" button
2. Adapter generates a demo credential: `{"demo_key": "demo-secret-key-<timestamp>"}`
3. Credential is saved and ready to use

---

## Installation

### Adding to llm-router

Add to your llm-router's `adapters.conf`:

```
github.com/TheSlopMachine/llm-router-adapter-demo main
```

Then rebuild llm-router:

```bash
make build
```

### For Development

```bash
go get github.com/TheSlopMachine/llm-router-adapter-demo@main
```

`@main` tracks the current tip of the `main` branch. Go still writes the resolved commit as a pseudo-version in `go.mod`, so rerun `go get ...@main` when you want newer code.

---

## Usage

### Basic Usage

1. Start llm-router with the demo adapter configured
2. Navigate to the dashboard at `http://localhost:8080`
3. Add a demo provider (type: `demo`)
4. Add credentials using the auth flow wizard
5. Create a router token
6. Make requests to demo models:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <your-router-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "demo/hello-model",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Testing Error Scenarios

Use the test models to verify error handling:

```bash
# Test rate limiting
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"model": "demo/rate-limit-model", "messages": [{"role": "user", "content": "test"}]}'

# Test quota exceeded
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"model": "demo/quota-model", "messages": [{"role": "user", "content": "test"}]}'

# Test authentication error
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"model": "demo/auth-error-model", "messages": [{"role": "user", "content": "test"}]}'
```

### Testing Streaming

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "demo/hello-model",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

---

## Code Structure

### adapter.go

Main adapter implementation with:
- **Core adapter methods** - All 10 required interface methods
- **Auth flow handler** - `DemoAuthFlow` implementation
- **Rate limiting logic** - `checkRateLimit()` helper
- **Test configuration** - `TestConfig` for custom behaviors
- **Helper functions** - `splitWords()`, `extractModelName()`

### doc.go

Comprehensive guide covering:
- **Quick start** - Creating a new adapter from scratch
- **Core adapter interface** - Detailed explanation of each method
- **Authentication flow handler** - State machine model and patterns
- **Authentication patterns** - Simple API key, OAuth2, multi-step flows
- **UI guidelines** - CSS classes, form wrapping, error handling
- **Best practices** - State management, security, user experience
- **Error handling** - Credential rotation patterns
- **Common pitfalls** - What to avoid

---

## Implementation Guide

### Using as a Template

1. **Copy the package structure**:
   ```
   your-adapter/
   ├── adapter.go    # Core implementation
   ├── doc.go        # Documentation (optional)
   ├── go.mod
   └── go.sum
   ```

2. **Update TypeKey()**:
   ```go
   func (a *Adapter) TypeKey() string { return "yourprovider" }
   ```

3. **Implement Complete()**:
   - Extract credentials from `cred.Data`
   - Make HTTP request to your provider's API
   - Transform response to `sdk.ChatCompletionResponse`
   - Return `*sdk.ProviderError` for retryable errors

4. **Implement CompleteStream()**:
   - Similar to `Complete()` but write SSE chunks to `w`
   - Format: `"data: {json}\n\n"`

5. **Implement ValidateCredentials()**:
   - Check for required fields
   - Validate formats
   - Optionally test the credential

6. **Implement GetModelInfos()**:
   - Fetch from provider API or return hardcoded values
   - Include rate limits and context window info

7. **Optional: Implement GetAuthFlow()**:
   - Return `nil` for manual credential entry
   - Return `AuthFlowHandler` for wizard-based auth

8. **Register in init()**:
   ```go
   func init() {
       sdk.Register(&Adapter{})
   }
   ```

### Key Implementation Details

#### Error Handling

Always return `*sdk.ProviderError` for retryable errors:

```go
// Rate limit
if resp.StatusCode == 429 {
    retryAfter := time.Now().Add(60 * time.Second)
    return nil, &sdk.ProviderError{
        StatusCode: 429,
        Message:    "rate limit exceeded",
        Type:       sdk.ErrorTypeRateLimit,
        RetryAfter: &retryAfter,
    }
}

// Quota exceeded (MUST have RetryAfter)
if quotaExceeded {
    resetAt := time.Now().Add(24 * time.Hour)
    return nil, &sdk.ProviderError{
        StatusCode: 429,
        Message:    "quota exceeded",
        Type:       sdk.ErrorTypeQuotaExceeded,
        RetryAfter: &resetAt, // Required!
    }
}
```

#### Streaming

Write SSE chunks in the correct format:

```go
chunk := sdk.StreamChunk{
    ID:      "id-" + fmt.Sprintf("%d", time.Now().Unix()),
    Object:  "chat.completion.chunk",
    Created: time.Now().Unix(),
    Model:   string(req.Model),
    Choices: []sdk.StreamChunkChoice{
        {
            Index: 0,
            Delta: sdk.ChatMessage{
                Role:    "assistant",
                Content: "word ",
            },
            FinishReason: nil,
        },
    },
}

data, _ := json.Marshal(chunk)
fmt.Fprintf(w, "data: %s\n\n", data)
```

#### Context Cancellation

Always check `ctx.Done()` for client disconnection:

```go
select {
case <-ctx.Done():
    return ctx.Err()
default:
    // Continue processing
}
```

---

## Testing

### Test Configuration

The adapter supports `TestConfig` for custom behaviors:

```go
adapter := demo.NewTestAdapter(demo.TestConfig{
    DisableRateLimit: true,
    ResponseFunc: func(req *sdk.ChatCompletionRequest) string {
        return "Custom response"
    },
    ModelBehaviors: map[string]demo.ModelBehavior{
        "custom-model": {
            Response: "Model-specific response",
            StreamDelay: 50 * time.Millisecond,
        },
    },
})
```

### Example Test Scenarios

1. **Test credential rotation on rate limit**:
   - Add multiple demo credentials
   - Make 11+ requests to `demo/hello-model`
   - Verify automatic credential rotation

2. **Test quota exhaustion**:
   - Make 51+ requests with a single credential
   - Verify quota exceeded error
   - Verify credential is deprioritized

3. **Test streaming**:
   - Request `demo/hello-model` with `"stream": true`
   - Verify SSE format
   - Verify word-by-word delivery

4. **Test error models**:
   - Use each test model (`rate-limit-model`, `quota-model`, etc.)
   - Verify correct error types and status codes

---

## Documentation

### Inline Comments

Every method in `adapter.go` includes comprehensive inline documentation explaining:
- What the method does
- When it's called
- Implementation steps
- Best practices
- Error handling

### doc.go Guide

The `doc.go` file contains a complete guide with:
- **Overview** - What adapters are and how they work
- **Quick start** - Step-by-step adapter creation
- **Core adapter interface** - Detailed method explanations
- **Authentication flow handler** - State machine model
- **Authentication patterns** - 3 complete examples (API key, OAuth2, multi-step)
- **UI guidelines** - CSS classes, form wrapping, error handling
- **Best practices** - State management, security, UX
- **Common pitfalls** - What to avoid

---

## Resources

- **SDK**: [github.com/TheSlopMachine/llm-router-sdk](https://github.com/TheSlopMachine/llm-router-sdk)
- **Main Router**: [github.com/TheSlopMachine/llm-router](https://github.com/TheSlopMachine/llm-router)
- **Documentation**: See [doc.go](doc.go) for comprehensive guide

---

## License

MIT
