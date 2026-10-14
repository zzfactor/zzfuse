package repository

import (
	"testing"
)

func TestDefaultTokenizer(t *testing.T) {
	tokenizer := NewDefaultTokenizer()

	t.Run("basic_tokenization", func(t *testing.T) {
		text := "Hello world this is a test"
		tokens, err := tokenizer.CountTokens(text)
		if err != nil {
			t.Errorf("Tokenization failed: %v", err)
		}

		// Should return a positive number of tokens
		if tokens <= 0 {
			t.Errorf("Expected positive token count, got %d", tokens)
		}
	})

	t.Run("empty_text", func(t *testing.T) {
		text := ""
		tokens, err := tokenizer.CountTokens(text)
		if err != nil {
			t.Errorf("Tokenization failed: %v", err)
		}

		if tokens != 0 {
			t.Errorf("Expected 0 tokens for empty text, got %d", tokens)
		}
	})

	t.Run("complex_text", func(t *testing.T) {
		text := "This is a more complex sentence with punctuation, numbers (123), and special characters: @#$%"
		tokens, err := tokenizer.CountTokens(text)
		if err != nil {
			t.Errorf("Tokenization failed: %v", err)
		}

		if tokens <= 0 {
			t.Errorf("Expected positive token count, got %d", tokens)
		}
	})

	t.Run("tokenizer_name", func(t *testing.T) {
		if tokenizer.GetName() != "default-tokenizer" {
			t.Errorf("Expected 'default-tokenizer', got: %s", tokenizer.GetName())
		}
	})
}

func TestSimpleTokenizer(t *testing.T) {
	tokenizer := NewSimpleTokenizer()

	t.Run("basic_tokenization", func(t *testing.T) {
		text := "Hello world this is a test"
		tokens, err := tokenizer.CountTokens(text)
		if err != nil {
			t.Errorf("Tokenization failed: %v", err)
		}

		expected := 6 // "Hello", "world", "this", "is", "a", "test"
		if tokens != expected {
			t.Errorf("Expected %d tokens, got %d", expected, tokens)
		}
	})

	t.Run("empty_text", func(t *testing.T) {
		text := ""
		tokens, err := tokenizer.CountTokens(text)
		if err != nil {
			t.Errorf("Tokenization failed: %v", err)
		}

		if tokens != 0 {
			t.Errorf("Expected 0 tokens for empty text, got %d", tokens)
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		text := "   \n\t  "
		tokens, err := tokenizer.CountTokens(text)
		if err != nil {
			t.Errorf("Tokenization failed: %v", err)
		}

		if tokens != 0 {
			t.Errorf("Expected 0 tokens for whitespace-only text, got %d", tokens)
		}
	})

	t.Run("tokenizer_name", func(t *testing.T) {
		if tokenizer.GetName() != "simple-whitespace" {
			t.Errorf("Expected 'simple-whitespace', got: %s", tokenizer.GetName())
		}
	})
}

func TestTokenBudget(t *testing.T) {
	tokenizer := NewDefaultTokenizer()
	maxTokens := 1000
	reservedTokens := 100

	t.Run("create_token_budget", func(t *testing.T) {
		budget := NewTokenBudget(tokenizer, maxTokens, reservedTokens)
		if budget == nil {
			t.Error("Token budget should not be nil")
		}

		if budget.maxTokens != maxTokens {
			t.Errorf("Expected max tokens %d, got %d", maxTokens, budget.maxTokens)
		}

		if budget.reserved != reservedTokens {
			t.Errorf("Expected reserved tokens %d, got %d", reservedTokens, budget.reserved)
		}
	})

	t.Run("get_available_tokens", func(t *testing.T) {
		budget := NewTokenBudget(tokenizer, maxTokens, reservedTokens)
		available := budget.GetAvailableTokens()

		expected := maxTokens - reservedTokens
		if available != expected {
			t.Errorf("Expected %d available tokens, got %d", expected, available)
		}
	})

	t.Run("reserve_tokens", func(t *testing.T) {
		budget := NewTokenBudget(tokenizer, maxTokens, reservedTokens)
		content := "This is a test content"

		tokens, err := budget.ReserveTokens(content)
		if err != nil {
			t.Errorf("Failed to reserve tokens: %v", err)
		}

		if tokens <= 0 {
			t.Errorf("Expected positive token count, got %d", tokens)
		}

		// Check that used tokens increased
		if budget.usedTokens != tokens {
			t.Errorf("Expected used tokens %d, got %d", tokens, budget.usedTokens)
		}
	})

	t.Run("exceed_budget", func(t *testing.T) {
		budget := NewTokenBudget(tokenizer, 5, 0) // Very small budget
		largeContent := "This is a very large content that should definitely exceed the small token budget of only 5 tokens. " +
			"It contains many more words than the budget allows. " +
			"Adding even more content to ensure we exceed the budget limit. " +
			"This should be more than enough to trigger the budget limit."

		tokens, err := budget.ReserveTokens(largeContent)
		if err == nil {
			t.Error("Expected error when exceeding budget")
		}

		// Should return available tokens even when exceeding
		if tokens <= 0 {
			t.Errorf("Expected positive available tokens, got %d", tokens)
		}
	})

	t.Run("reset_budget", func(t *testing.T) {
		budget := NewTokenBudget(tokenizer, maxTokens, reservedTokens)
		content := "Test content"

		_, err := budget.ReserveTokens(content)
		if err != nil {
			t.Errorf("Failed to reserve tokens: %v", err)
		}

		// Reset should clear used tokens
		budget.Reset()
		if budget.usedTokens != 0 {
			t.Errorf("Expected 0 used tokens after reset, got %d", budget.usedTokens)
		}
	})

	t.Run("get_usage_stats", func(t *testing.T) {
		budget := NewTokenBudget(tokenizer, maxTokens, reservedTokens)
		stats := budget.GetUsageStats()

		requiredFields := []string{"max_tokens", "reserved", "used", "available", "usage_percent", "tokenizer_name"}
		for _, field := range requiredFields {
			if _, exists := stats[field]; !exists {
				t.Errorf("Missing required field in usage stats: %s", field)
			}
		}
	})
}

func TestGetDefaultTokenizer(t *testing.T) {
	t.Run("get_default_tokenizer", func(t *testing.T) {
		tokenizer := GetDefaultTokenizer()
		if tokenizer == nil {
			t.Error("Default tokenizer should not be nil")
		}

		name := tokenizer.GetName()
		if name != "default-tokenizer" {
			t.Errorf("Expected 'default-tokenizer', got: %s", name)
		}

		// Test that it can count tokens
		text := "Hello world"
		tokens, err := tokenizer.CountTokens(text)
		if err != nil {
			t.Errorf("Tokenization failed: %v", err)
		}

		if tokens <= 0 {
			t.Errorf("Expected positive token count, got %d", tokens)
		}
	})
}
