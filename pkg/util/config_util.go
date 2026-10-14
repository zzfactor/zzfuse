package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// On package init, load .env from project root if present
func init() {
	_ = godotenv.Load()
}

const defaultSecretsDir = "./secrets/"

const (
	DefaultDBMaxOpenConns    = 20
	DefaultDBMaxIdleConns    = 10
	DefaultDBConnMaxLifetime = 30 * time.Minute
)

// GetSecret resolves a secret reference string.
//
// Supported formats:
//   - dev:literal_value         (returns the literal value, for development/testing only)
//   - env:ENV_VAR               (environment variable)
//   - vault:secret/path#key     (HashiCorp Vault or other secret manager)
//   - file:/path/to/secret      (read secret from file; if #key is present, extract key from JSON)
//   - file:/path/to/secret#key  (read secret from file, extract key from JSON)
//   - k8s:namespace/secret#key  (Kubernetes secret)
//   - custom:provider:params    (custom plugin/provider)
//   - ENV_VAR                   (fallback: treat as environment variable)
//
// For now, only 'dev:', 'env:', and 'file:' (with optional #key for JSON) are implemented. Others are stubs for future support.
func GetSecret(secretRef string) string {
	refLower := strings.ToLower(secretRef)
	if strings.HasPrefix(refLower, "dev:") {
		return secretRef[len("dev:"):]
	}
	if strings.HasPrefix(refLower, "env:") {
		return os.Getenv(strings.TrimSpace(secretRef[len("env:"):]))
	}
	if strings.HasPrefix(refLower, "file:") {
		pathKey := strings.TrimSpace(secretRef[len("file:"):])
		var path, key string
		if idx := strings.Index(pathKey, "#"); idx != -1 {
			path = pathKey[:idx]
			key = pathKey[idx+1:]
		} else {
			path = pathKey
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(defaultSecretsDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		val := strings.TrimSpace(string(data))
		if key == "" {
			return val
		}
		// Try to parse as JSON and extract the key
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err == nil {
			if v, ok := m[key]; ok {
				if s, ok := v.(string); ok {
					return s
				}
				// If not a string, return JSON representation
				b, _ := json.Marshal(v)
				return string(b)
			}
		}
		return "" // Key not found or not JSON
	}
	if strings.HasPrefix(refLower, "vault:") {
		return "" // Not yet implemented
	}
	if strings.HasPrefix(refLower, "k8s:") {
		return "" // Not yet implemented
	}
	if strings.HasPrefix(refLower, "custom:") {
		return "" // Not yet implemented
	}
	return os.Getenv(strings.TrimSpace(secretRef))
}

// GetSecretFromMetadata retrieves and resolves a secret reference from a metadata map by key.
// If the key is present and is a string, it resolves the secret using GetSecret (supports all prefixes).
// Returns an empty string if the key is not present or not a string.
func GetSecretFromMetadata(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	if secrets, ok := metadata["secrets"].(map[string]interface{}); ok {
		if ref, ok := secrets[key].(string); ok {
			return GetSecret(ref)
		}
	}
	return ""
}

func GetEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
func GetEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func GetEnvString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ToInt attempts to convert an interface{} to int
func ToInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i, true
		}
	}
	return 0, false
}

// ToDuration attempts to convert an interface{} to time.Duration
func ToDuration(v interface{}) (time.Duration, bool) {
	switch val := v.(type) {
	case time.Duration:
		return val, true
	case int:
		return time.Duration(val) * time.Second, true
	case int64:
		return time.Duration(val) * time.Second, true
	case float64:
		return time.Duration(val) * time.Second, true
	case string:
		if d, err := time.ParseDuration(val); err == nil {
			return d, true
		}
	}
	return 0, false
}
