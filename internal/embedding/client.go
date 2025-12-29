// Package embedding provides the embedding client for text-to-vector conversion.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// Client defines the interface for embedding operations.
type Client interface {
	// Embed converts a single text to a vector.
	Embed(ctx context.Context, text string) ([]float64, error)

	// EmbedBatch converts multiple texts to vectors.
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
}

// Config holds the configuration for the embedding client.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// OpenAIClient implements the Client interface using OpenAI-compatible API.
type OpenAIClient struct {
	config     Config
	httpClient *http.Client
}

// embeddingRequest represents the request body for the embedding API.
type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

// embeddingResponse represents the response from the embedding API.
type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *apiError       `json:"error,omitempty"`
}

type embeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// NewOpenAIClient creates a new OpenAI-compatible embedding client.
func NewOpenAIClient(cfg Config) *OpenAIClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &OpenAIClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}


// Embed converts a single text to a vector.
// Implements Requirements 3.1, 3.5
func (c *OpenAIClient) Embed(ctx context.Context, text string) ([]float64, error) {
	vectors, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return vectors[0], nil
}

// EmbedBatch converts multiple texts to vectors with retry logic.
// Implements Requirements 3.1, 3.5
func (c *OpenAIClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		vectors, err := c.doEmbedRequest(ctx, texts)
		if err == nil {
			return vectors, nil
		}

		lastErr = err

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("embedding request failed after %d retries: %w", maxRetries, lastErr)
}

// doEmbedRequest performs a single embedding API request.
func (c *OpenAIClient) doEmbedRequest(ctx context.Context, texts []string) ([][]float64, error) {
	reqBody := embeddingRequest{
		Input: texts,
		Model: c.config.Model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.config.BaseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp embeddingResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(embResp.Data) != len(texts) {
		return nil, fmt.Errorf("unexpected number of embeddings: got %d, expected %d", len(embResp.Data), len(texts))
	}

	// Sort by index to ensure correct order
	vectors := make([][]float64, len(texts))
	for _, d := range embResp.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("invalid embedding index: %d", d.Index)
		}
		vectors[d.Index] = d.Embedding
	}

	return vectors, nil
}

// CosineSimilarity calculates the cosine similarity between two vectors.
// Returns a value in the range [-1, 1].
// Handles zero vectors by returning 0.
// Implements Requirements 3.6
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	// Handle zero vectors
	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
