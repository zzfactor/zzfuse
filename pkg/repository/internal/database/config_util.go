package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// unmarshalJSONField safely unmarshals a JSON string into a map, handling empty or null values
func unmarshalJSONField(jsonStr string, fieldName string) (map[string]interface{}, error) {
	if jsonStr == "" || jsonStr == "null" {
		return make(map[string]interface{}), nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s: %w", fieldName, err)
	}
	return result, nil
}

// unmarshalJSONFieldGeneric safely unmarshals a JSON string into any type, handling empty or null values
func unmarshalJSONFieldGeneric[T any](jsonStr string, fieldName string) (T, error) {
	var result T
	if jsonStr == "" || jsonStr == "null" {
		return result, nil
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal %s: %w", fieldName, err)
	}
	return result, nil
}

// tableExists checks if a table exists in the specified schema
func tableExists(ctx context.Context, db *sql.DB, schema, tableName string) (bool, error) {
	query := `
	SELECT EXISTS (
		SELECT FROM information_schema.tables 
		WHERE table_schema = $1 AND table_name = $2
	)`
	var exists bool
	err := db.QueryRowContext(ctx, query, schema, tableName).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
