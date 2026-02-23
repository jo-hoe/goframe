package commandstructure

import (
	"fmt"
	"strconv"
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

/* no-op */

/* Reinsert GetIntParam and add GetFloatParam, then original GetBoolParam */

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

// GetFloatParam safely extracts a float64 parameter from the params map
func GetFloatParam(params map[string]any, key string, defaultValue float64) float64 {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case string:
			s := strings.TrimSpace(v)
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
	}
	return defaultValue
}

// GetBoolParam safely extracts a bool parameter from the params map
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
		case bool:
			return v
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
