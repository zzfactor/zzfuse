package repository

import (
	"fmt"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

// Tokenizer interface provides token counting functionality
type Tokenizer interface {
	CountTokens(text string) (int, error)
	GetName() string
}

// DefaultTokenizer provides accurate token counting using GPT-4 compatible encoding
// This works well for most modern embedding models
type DefaultTokenizer struct{}

// NewDefaultTokenizer creates a new default tokenizer
func NewDefaultTokenizer() *DefaultTokenizer {
	return &DefaultTokenizer{}
}

func (t *DefaultTokenizer) CountTokens(text string) (int, error) {
	// Use tiktoken library for accurate token counting (cl100k_base)
	encoding, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return 0, fmt.Errorf("failed to get encoding: %w", err)
	}

	tokens := encoding.Encode(text, nil, nil)
	return len(tokens), nil
}

func (t *DefaultTokenizer) GetName() string {
	return "default-tokenizer"
}

// SimpleTokenizer provides basic whitespace-based token counting as fallback
type SimpleTokenizer struct{}

// NewSimpleTokenizer creates a new simple tokenizer
func NewSimpleTokenizer() *SimpleTokenizer {
	return &SimpleTokenizer{}
}

func (t *SimpleTokenizer) CountTokens(text string) (int, error) {
	// Simple whitespace-based tokenization
	tokens := strings.Fields(text)
	return len(tokens), nil
}

func (t *SimpleTokenizer) GetName() string {
	return "simple-whitespace"
}

// TokenBudget manages token allocation for content optimization
type TokenBudget struct {
	tokenizer  Tokenizer
	maxTokens  int
	reserved   int // Reserved tokens for safety margin
	usedTokens int
}

// NewTokenBudget creates a new token budget manager
func NewTokenBudget(tokenizer Tokenizer, maxTokens int, reservedTokens int) *TokenBudget {
	if reservedTokens <= 0 {
		reservedTokens = int(float64(maxTokens) * 0.1) // 10% safety margin
	}

	return &TokenBudget{
		tokenizer:  tokenizer,
		maxTokens:  maxTokens,
		reserved:   reservedTokens,
		usedTokens: 0,
	}
}

// GetAvailableTokens returns the number of tokens still available
func (tb *TokenBudget) GetAvailableTokens() int {
	available := tb.maxTokens - tb.reserved - tb.usedTokens
	if available < 0 {
		return 0
	}
	return available
}

// ReserveTokens reserves tokens for a piece of content
func (tb *TokenBudget) ReserveTokens(content string) (int, error) {
	tokens, err := tb.tokenizer.CountTokens(content)
	if err != nil {
		return 0, err
	}

	available := tb.GetAvailableTokens()
	if tokens > available {
		return available, fmt.Errorf("content requires %d tokens but only %d available", tokens, available)
	}

	tb.usedTokens += tokens
	return tokens, nil
}

// GetUsageStats returns current token usage statistics
func (tb *TokenBudget) GetUsageStats() map[string]interface{} {
	return map[string]interface{}{
		"max_tokens":     tb.maxTokens,
		"reserved":       tb.reserved,
		"used":           tb.usedTokens,
		"available":      tb.GetAvailableTokens(),
		"usage_percent":  float64(tb.usedTokens) / float64(tb.maxTokens) * 100,
		"tokenizer_name": tb.tokenizer.GetName(),
	}
}

// Reset resets the token budget
func (tb *TokenBudget) Reset() {
	tb.usedTokens = 0
}

// GetDefaultTokenizer returns the default tokenizer
func GetDefaultTokenizer() Tokenizer {
	return NewDefaultTokenizer()
}
