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
	configContent := `
port: 8080
database:
	type: "sqlite"
	connectionString: "test-connection-string"
imageTargetType: "png"`
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

	if config.Database.ConnectionString != "test-connection-string" {
		t.Errorf("Expected connectionString to be 'test-connection-string', got '%s'", config.Database.ConnectionString)
	}

	if config.Database.Type != "sqlite" {
		t.Errorf("Expected database type to be 'sqlite', got '%s'", config.Database.Type)
	}

	if config.ImageTargetType != "png" {
		t.Errorf("Expected imageTargetType to be 'png', got '%s'", config.ImageTargetType)
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

func TestLoadConfig_WithProcessors(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `port: 8080
connectionString: "test-connection-string"
processors:
  - name: OrientationProcessor
    orientation: portrait
  - name: CropProcessor
    height: 1600
    width: 1200`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(config.Processors) != 2 {
		t.Fatalf("Expected 2 processors, got %d", len(config.Processors))
	}

	// Verify first processor
	if config.Processors[0].Name != "OrientationProcessor" {
		t.Errorf("Expected first processor name to be 'OrientationProcessor', got '%s'", config.Processors[0].Name)
	}
	if orientation, ok := config.Processors[0].Params["orientation"].(string); !ok || orientation != "portrait" {
		t.Errorf("Expected orientation to be 'portrait', got '%v'", config.Processors[0].Params["orientation"])
	}

	// Verify second processor
	if config.Processors[1].Name != "CropProcessor" {
		t.Errorf("Expected second processor name to be 'CropProcessor', got '%s'", config.Processors[1].Name)
	}
	if height, ok := config.Processors[1].Params["height"].(int); !ok || height != 1600 {
		t.Errorf("Expected height to be 1600, got '%v'", config.Processors[1].Params["height"])
	}
	if width, ok := config.Processors[1].Params["width"].(int); !ok || width != 1200 {
		t.Errorf("Expected width to be 1200, got '%v'", config.Processors[1].Params["width"])
	}
}

func TestLoadConfig_EmptyProcessorName(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `port: 8080
connectionString: "test-connection-string"
processors:
  - name: ""
    orientation: portrait`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for empty processor name, got nil")
	}
}

func TestLoadConfig_DuplicateProcessorName(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `port: 8080
connectionString: "test-connection-string"
processors:
  - name: OrientationProcessor
    orientation: portrait
  - name: OrientationProcessor
    orientation: landscape`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for duplicate processor name, got nil")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `port: 8080
connectionString: "test-connection-string"
processors:
  - name: OrientationProcessor
    orientation: portrait
  invalid yaml syntax here`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for invalid YAML, got nil")
	}
}
