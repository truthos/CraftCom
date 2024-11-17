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
	"context"
	"fmt"
	"os"
	"sync"

	"craftcom/pkg/gemini"
	"craftcom/pkg/types"
)

// Version represents the library version
const Version = "0.1.0"

// Terma represents the main library instance
type Terma struct {
	config    *Config
	providers map[string]types.Provider
	history   []types.CommandHistory
	mu        sync.RWMutex
}

// New creates a new Terma instance
func New(configPath string) (*Terma, error) {
	// Load configuration
	config, err := LoadConfigFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	t := &Terma{
		config:    config,
		providers: make(map[string]types.Provider),
		history:   make([]types.CommandHistory, 0, config.HistorySize),
	}

	// Initialize providers
	if err := t.initializeProviders(); err != nil {
		return nil, fmt.Errorf("failed to initialize providers: %w", err)
	}

	return t, nil
}

// initializeProviders sets up AI providers based on configuration
func (t *Terma) initializeProviders() error {
	ctx := context.Background()

	for name, config := range t.config.Providers {
		if !config.Enabled {
			continue
		}

		switch name {
		case "gemini":
			provider, err := gemini.NewProvider(
				ctx,
				config.APIKey,
				t.config.SystemPrompt,
			)
			if err != nil {
				return fmt.Errorf("failed to initialize Gemini provider: %w", err)
			}
			t.providers[name] = provider

		// Add more providers here as needed
		default:
			return fmt.Errorf("unknown provider: %s", name)
		}
	}

	return nil
}

// Chat creates a new chat session
func (t *Terma) Chat(ctx context.Context) (types.Chat, error) {
	provider, err := t.getProvider(t.config.DefaultProvider)
	if err != nil {
		return nil, err
	}

	return provider.Chat(ctx, t.config.DefaultModel)
}

// ChatWithProvider creates a chat session with a specific provider
func (t *Terma) ChatWithProvider(ctx context.Context, providerName, model string) (types.Chat, error) {
	provider, err := t.getProvider(providerName)
	if err != nil {
		return nil, err
	}

	return provider.Chat(ctx, model)
}

// Execute runs a command through the AI assistant
func (t *Terma) Execute(ctx context.Context, input string) (*types.CommandHistory, error) {
	// Get chat instance
	chat, err := t.Chat(ctx)
	if err != nil {
		return nil, err
	}
	defer chat.Close()

	// Generate command
	resp, err := chat.Send(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to generate command: %w", err)
	}

	// Validate command
	if err := t.config.ValidateCommand(resp.Code); err != nil {
		return nil, err
	}

	// Execute command
	result := &types.CommandHistory{
		Command: resp.Code,
		Output:  resp.FullOutput,
	}

	// Add to history
	t.addToHistory(result)

	return result, nil
}

// ExecuteWithFiles runs a command with file inputs
func (t *Terma) ExecuteWithFiles(ctx context.Context, input string, files []string) (*types.CommandHistory, error) {
	chat, err := t.Chat(ctx)
	if err != nil {
		return nil, err
	}
	defer chat.Close()

	resp, err := chat.SendWithFiles(ctx, input, files)
	if err != nil {
		return nil, fmt.Errorf("failed to generate command: %w", err)
	}

	if err := t.config.ValidateCommand(resp.Code); err != nil {
		return nil, err
	}

	result := &types.CommandHistory{
		Command: resp.Code,
		Output:  resp.FullOutput,
	}

	t.addToHistory(result)

	return result, nil
}

// GetHistory returns command history
func (t *Terma) GetHistory() []types.CommandHistory {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]types.CommandHistory{}, t.history...)
}

// ClearHistory clears command history
func (t *Terma) ClearHistory() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.history = make([]types.CommandHistory, 0, t.config.HistorySize)
}

// GetProvider returns a provider by name
func (t *Terma) getProvider(name string) (types.Provider, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	provider, ok := t.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", name)
	}

	return provider, nil
}

// addToHistory adds a command to history
func (t *Terma) addToHistory(cmd *types.CommandHistory) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.history = append(t.history, *cmd)
	if len(t.history) > t.config.HistorySize {
		t.history = t.history[1:]
	}
}

// Close cleans up resources
func (t *Terma) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, provider := range t.providers {
		if err := provider.Close(); err != nil {
			return fmt.Errorf("failed to close provider: %w", err)
		}
	}

	return nil
}

// GetSystemInfo returns current system information
func (t *Terma) GetSystemInfo() types.SystemInfo {
	wd, _ := os.Getwd()
	homeDir, _ := os.UserHomeDir()

	return types.SystemInfo{
		OS:          os.Getenv("GOOS"),
		Shell:       t.config.Shell,
		User:        os.Getenv("USER"),
		HomeDir:     homeDir,
		WorkingDir:  wd,
		Environment: t.config.Environment,
	}
}
