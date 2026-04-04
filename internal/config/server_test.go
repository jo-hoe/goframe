package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServerConfig_Success(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
port: 8080
imageTargetType: "png"
database:
  type: "redis"
  connectionString: "test-connection-string"`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	config, err := LoadServerConfig(configPath)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}

	if config.Port != 8080 {
		t.Errorf("Expected port to be 8080, got %d", config.Port)
	}

	if config.Database.ConnectionString != "test-connection-string" {
		t.Errorf("Expected connectionString to be 'test-connection-string', got '%s'", config.Database.ConnectionString)
	}

	if config.Database.Type != "redis" {
		t.Errorf("Expected database type to be 'redis', got '%s'", config.Database.Type)
	}
}

func TestLoadServerConfig_FileNotFound(t *testing.T) {
	nonExistentPath := "/path/that/does/not/exist/config.yaml"

	config, err := LoadServerConfig(nonExistentPath)

	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}

	if config != nil {
		t.Error("Expected config to be nil when file doesn't exist")
	}
}

func TestLoadServerConfig_WithCommands(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `port: 8080
connectionString: "test-connection-string"
commands:
  - name: OrientationCommand
    orientation: portrait
  - name: CropCommand
    height: 1600
    width: 1200`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	config, err := LoadServerConfig(configPath)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if len(config.Commands) != 2 {
		t.Fatalf("Expected 2 commands, got %d", len(config.Commands))
	}

	if config.Commands[0].Name != "OrientationCommand" {
		t.Errorf("Expected first command name to be 'OrientationCommand', got '%s'", config.Commands[0].Name)
	}
	if orientation, ok := config.Commands[0].Params["orientation"].(string); !ok || orientation != "portrait" {
		t.Errorf("Expected orientation to be 'portrait', got '%v'", config.Commands[0].Params["orientation"])
	}

	if config.Commands[1].Name != "CropCommand" {
		t.Errorf("Expected second command name to be 'CropCommand', got '%s'", config.Commands[1].Name)
	}
	if height, ok := config.Commands[1].Params["height"].(int); !ok || height != 1600 {
		t.Errorf("Expected height to be 1600, got '%v'", config.Commands[1].Params["height"])
	}
	if width, ok := config.Commands[1].Params["width"].(int); !ok || width != 1200 {
		t.Errorf("Expected width to be 1200, got '%v'", config.Commands[1].Params["width"])
	}
}

func TestLoadServerConfig_EmptyCommandName(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `port: 8080
connectionString: "test-connection-string"
commands:
  - name: ""
    orientation: portrait`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadServerConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for empty command name, got nil")
	}
}

func TestLoadServerConfig_DuplicateCommandName(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `port: 8080
connectionString: "test-connection-string"
commands:
  - name: OrientationCommand
    orientation: portrait
  - name: OrientationCommand
    orientation: landscape`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadServerConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for duplicate command name, got nil")
	}
}

func TestLoadServerConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `port: 8080
connectionString: "test-connection-string"
commands:
  - name: OrientationCommand
    orientation: portrait
  invalid yaml syntax here`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadServerConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for invalid YAML, got nil")
	}
}
