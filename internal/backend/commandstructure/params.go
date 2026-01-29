package commandstructure

import (
	"fmt"
	"strings"
)

// GetStringParam safely extracts a string parameter from the params map
func GetStringParam(params map[string]any, key string, defaultValue string) string {
	if val, ok := params[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// GetIntParam safely extracts an int parameter from the params map
func GetIntParam(params map[string]any, key string, defaultValue int) int {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

// GetBoolParam safely extracts a bool parameter from the params map
// Accepts common truthy/falsey representations: true/false, 1/0, yes/no, on/off (case-insensitive).
func GetBoolParam(params map[string]any, key string, defaultValue bool) bool {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case string:
			s := strings.ToLower(strings.TrimSpace(v))
			switch s {
			case "true":
				return true
			case "false":
				return false
			default:
				return defaultValue
			}
		}
	}
	return defaultValue
}

// ValidateRequiredParams checks that all required parameters are present
func ValidateRequiredParams(params map[string]any, required []string) error {
	for _, key := range required {
		if _, ok := params[key]; !ok {
			return fmt.Errorf("missing required parameter: %s", key)
		}
	}
	return nil
}
