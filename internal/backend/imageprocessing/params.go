package imageprocessing

import (
	"fmt"
)

// getStringParam safely extracts a string parameter from the params map
func getStringParam(params map[string]any, key string, defaultValue string) string {
	if val, ok := params[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// getIntParam safely extracts an int parameter from the params map
func getIntParam(params map[string]any, key string, defaultValue int) int {
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

// validateRequiredParams checks that all required parameters are present
func validateRequiredParams(params map[string]any, required []string) error {
	for _, key := range required {
		if _, ok := params[key]; !ok {
			return fmt.Errorf("missing required parameter: %s", key)
		}
	}
	return nil
}
