package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a valid config file
	configContent := `port: 8080
connectionString: "test-connection-string"`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test LoadConfig
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify the loaded configuration
	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}

	if config.Port != 8080 {
		t.Errorf("Expected port to be 8080, got %d", config.Port)
	}

	if config.ConnectionString != "test-connection-string" {
		t.Errorf("Expected connectionString to be 'test-connection-string', got '%s'", config.ConnectionString)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	// Test with a non-existent file
	nonExistentPath := "/path/that/does/not/exist/config.yaml"

	config, err := LoadConfig(nonExistentPath)

	// Expect an error
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}

	// Config should be nil
	if config != nil {
		t.Error("Expected config to be nil when file doesn't exist")
	}
}
