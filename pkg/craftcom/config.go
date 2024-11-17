// Copyright (c) 2024 TruthOS
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package libterma

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"craftcom/pkg/types"
)

// Config represents the global configuration
type Config struct {
	// Provider configurations
	Providers map[string]ProviderConfig `json:"providers"`

	// Default provider and model settings
	DefaultProvider string `json:"default_provider"`
	DefaultModel    string `json:"default_model"`

	// Terminal settings
	Shell        string            `json:"shell"`
	HistorySize  int               `json:"history_size"`
	WorkingDir   string            `json:"working_dir"`
	Environment  map[string]string `json:"environment"`
	SystemPrompt string            `json:"system_prompt"`

	// Safety settings
	DisallowedCommands []string `json:"disallowed_commands"`
	ProtectedPaths     []string `json:"protected_paths"`
	SafetyLevel        string   `json:"safety_level"`

	// File handling settings
	MaxFileSize      int64    `json:"max_file_size"`
	AllowedFileTypes []string `json:"allowed_file_types"`

	// Runtime settings
	Debug       bool `json:"debug"`
	Quiet       bool `json:"quiet"`
	ColorOutput bool `json:"color_output"`

	// Internal fields
	configPath string
	mu         sync.RWMutex
}

// ProviderConfig contains provider-specific settings
type ProviderConfig struct {
	APIKey      string            `json:"api_key"`
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	Models      []string          `json:"models"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature float32           `json:"temperature"`
	TopP        float32           `json:"top_p"`
	Settings    map[string]string `json:"settings"`
}

const (
	defaultConfigName  = ".craftcom.json"
	defaultHistorySize = 1000
	defaultSafetyLevel = "medium"
)

// DefaultConfig creates a new configuration with default values
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()

	return &Config{
		Providers: map[string]ProviderConfig{
			"gemini": {
				Name:    "gemini",
				Enabled: true,
				Models:  []string{"gemini-1.5-pro", "gemini-1.5-flash"},
				Settings: map[string]string{
					"temperature": "0.7",
					"top_p":       "0.95",
				},
			},
		},
		DefaultProvider: "gemini",
		DefaultModel:    "gemini-1.5-pro",
		Shell:           os.Getenv("SHELL"),
		HistorySize:     defaultHistorySize,
		WorkingDir:      homeDir,
		Environment:     map[string]string{},
		SystemPrompt:    defaultSystemPrompt,
		DisallowedCommands: []string{
			"rm -rf /",
			"mkfs",
			"dd if=/dev/zero",
		},
		ProtectedPaths: []string{
			"/etc",
			"/var",
			"/usr",
			"/boot",
			"/root",
		},
		SafetyLevel: defaultSafetyLevel,
		MaxFileSize: 100 * 1024 * 1024, // 100MB
		AllowedFileTypes: []string{
			".txt", ".md", ".pdf",
			".png", ".jpg", ".jpeg",
			".mp3", ".wav",
			".mp4",
		},
		ColorOutput: true,
	}
}

// LoadConfig loads the configuration from the default location
func LoadConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, types.ErrConfigurationf("failed to get home directory: %v", err)
	}

	configPath := filepath.Join(homeDir, defaultConfigName)
	return LoadConfigFromPath(configPath)
}

// LoadConfigFromPath loads the configuration from a specific path
func LoadConfigFromPath(path string) (*Config, error) {
	config := DefaultConfig()
	config.configPath = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default config if it doesn't exist
			return config, config.Save()
		}
		return nil, types.ErrConfigurationf("failed to read config: %v", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, types.ErrConfigurationf("failed to parse config: %v", err)
	}

	return config, nil
}

// Save writes the configuration to disk
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return types.ErrConfigurationf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(c.configPath, data, 0600); err != nil {
		return types.ErrConfigurationf("failed to write config: %v", err)
	}

	return nil
}

// GetProviderConfig returns configuration for a specific provider
func (c *Config) GetProviderConfig(name string) (ProviderConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	config, ok := c.Providers[name]
	if !ok {
		return ProviderConfig{}, types.ErrConfigurationf("provider not found: %s", name)
	}

	return config, nil
}

// UpdateProviderConfig updates configuration for a specific provider
func (c *Config) UpdateProviderConfig(name string, config ProviderConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Providers[name] = config
	return c.Save()
}

// ValidateCommand checks if a command is allowed
func (c *Config) ValidateCommand(cmd string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check against disallowed commands
	for _, disallowed := range c.DisallowedCommands {
		if cmd == disallowed {
			return types.ErrPermissionf("command is not allowed: %s", cmd)
		}
	}

	// Check protected paths
	for _, path := range c.ProtectedPaths {
		if containsPath(cmd, path) {
			return types.ErrPermissionf("command affects protected path: %s", path)
		}
	}

	return nil
}

// Helper function to check if a command contains a protected path
func containsPath(cmd, path string) bool {
	// Implement path checking logic
	return false
}

const defaultSystemPrompt = `You are a terminal assistant. Help users with:
1. Converting natural language to terminal commands
2. Explaining command functionality
3. Suggesting command improvements
4. Handling errors and providing fixes

Maintain system safety:
- No destructive commands
- Protect system directories
- Verify command safety
- Explain potential risks`
