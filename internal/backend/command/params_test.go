package command

import (
	"testing"
)

func TestGetStringParam(t *testing.T) {
	params := map[string]any{
		"key1": "value1",
		"key2": 123,
	}

	// Test existing string parameter
	if val := getStringParam(params, "key1", "default"); val != "value1" {
		t.Errorf("Expected 'value1', got '%s'", val)
	}

	// Test non-string parameter
	if val := getStringParam(params, "key2", "default"); val != "default" {
		t.Errorf("Expected 'default', got '%s'", val)
	}

	// Test non-existent parameter
	if val := getStringParam(params, "key3", "default"); val != "default" {
		t.Errorf("Expected 'default', got '%s'", val)
	}
}

func TestGetIntParam(t *testing.T) {
	params := map[string]any{
		"key1": 123,
		"key2": int64(456),
		"key3": float64(789),
		"key4": "not-an-int",
	}

	// Test int parameter
	if val := getIntParam(params, "key1", 0); val != 123 {
		t.Errorf("Expected 123, got %d", val)
	}

	// Test int64 parameter
	if val := getIntParam(params, "key2", 0); val != 456 {
		t.Errorf("Expected 456, got %d", val)
	}

	// Test float64 parameter
	if val := getIntParam(params, "key3", 0); val != 789 {
		t.Errorf("Expected 789, got %d", val)
	}

	// Test non-int parameter
	if val := getIntParam(params, "key4", 999); val != 999 {
		t.Errorf("Expected 999, got %d", val)
	}

	// Test non-existent parameter
	if val := getIntParam(params, "key5", 999); val != 999 {
		t.Errorf("Expected 999, got %d", val)
	}
}

func TestValidateRequiredParams(t *testing.T) {
	params := map[string]any{
		"param1": "value1",
		"param2": 123,
	}

	// Test all required params present
	err := validateRequiredParams(params, []string{"param1", "param2"})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test missing required param
	err = validateRequiredParams(params, []string{"param1", "param3"})
	if err == nil {
		t.Error("Expected error for missing required param")
	}

	// Test no required params
	err = validateRequiredParams(params, []string{})
	if err != nil {
		t.Errorf("Expected no error for empty required list, got %v", err)
	}
}
