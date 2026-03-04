package agwclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComplete_Success(t *testing.T) {
	want := CompletionResponse{
		ID:    "chatcmpl-123",
		Model: "gpt-4",
		Choices: []Choice{
			{Index: 0, Message: ChatMessage{Role: "assistant", Content: "Hello!"}},
		},
		Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, http.MethodPost, r.Method)

		var req CompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "gpt-4", req.Model)
		assert.False(t, req.Stream, "stream must be false")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})

	got, err := client.Complete(context.Background(), &CompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.Model, got.Model)
	assert.Len(t, got.Choices, 1)
	assert.Equal(t, "Hello!", got.Choices[0].Message.Content)
	assert.Equal(t, 15, got.Usage.TotalTokens)
}

func TestComplete_BaseURLTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CompletionResponse{
			ID:    "chatcmpl-slash",
			Model: "gpt-4o",
			Choices: []Choice{
				{Index: 0, Message: ChatMessage{Role: "assistant", Content: "ok"}},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		BaseURL:    srv.URL + "/", // trailing slash should not produce //v1/...
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})

	resp, err := client.Complete(context.Background(), &CompletionRequest{
		Model:    "gpt-4o",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "chatcmpl-slash", resp.ID)
}

func TestComplete_RetryOn500(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CompletionResponse{
			ID:    "chatcmpl-retry",
			Model: "gpt-4",
			Choices: []Choice{
				{Index: 0, Message: ChatMessage{Role: "assistant", Content: "OK"}},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
	})

	got, err := client.Complete(context.Background(), &CompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-retry", got.ID)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestComplete_RetryOn429(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CompletionResponse{ID: "chatcmpl-429"})
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 2,
	})

	got, err := client.Complete(context.Background(), &CompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-429", got.ID)
	assert.Equal(t, int32(2), attempts.Load())
}

func TestComplete_NoRetryOn400(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
	})

	_, err := client.Complete(context.Background(), &CompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	require.Error(t, err)
	assert.IsType(t, &NonRetryableError{}, err)
	assert.Equal(t, int32(1), attempts.Load(), "should not retry on 400")
}

func TestComplete_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Complete(ctx, &CompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestComplete_ExhaustedRetries(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("always fails"))
	}))
	defer srv.Close()

	client := NewClient(ClientConfig{
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 2,
	})

	_, err := client.Complete(context.Background(), &CompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
	assert.Equal(t, int32(3), attempts.Load())
}
