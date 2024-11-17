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

package types

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Response represents the AI model's response
type Response struct {
	Code       string                 `json:"code"`        // Generated command or code
	FullOutput string                 `json:"full_output"` // Full response with explanations
	Metadata   map[string]interface{} `json:"metadata"`    // Additional metadata
}

// Chat defines methods for chat-based interactions
type Chat interface {
	// Send sends a text message and returns a response
	Send(ctx context.Context, message string) (Response, error)

	// SendWithFiles sends a message with file attachments
	SendWithFiles(ctx context.Context, message string, files []string) (Response, error)

	// Close cleans up resources
	Close() error
}

// Provider defines methods that must be implemented by each AI provider
type Provider interface {
	// Chat creates a new chat session
	Chat(ctx context.Context, model string) (Chat, error)

	// ListModels returns available models
	ListModels(ctx context.Context) ([]string, error)

	// GetModelInfo returns model configuration
	GetModelInfo(model string) (ModelInfo, error)

	// ValidateConfig checks configuration
	ValidateConfig() error

	// Close cleans up resources
	Close() error
}

// ModelInfo contains model configuration and capabilities
type ModelInfo struct {
	Name             string        `json:"name"`
	InputTokenLimit  int           `json:"input_token_limit"`
	OutputTokenLimit int           `json:"output_token_limit"`
	RPM              int           `json:"rpm"`      // Requests per minute
	TPM              int           `json:"tpm"`      // Tokens per minute
	RPD              int           `json:"rpd"`      // Requests per day
	Features         []string      `json:"features"` // Supported features
	Timeout          time.Duration `json:"timeout"`  // Default timeout
	IsPaid           bool          `json:"is_paid"`
}

// FileProcessor handles different file types
type FileProcessor interface {
	// Process processes a file and returns its content
	Process(ctx context.Context, path string) (io.Reader, error)

	// SupportedTypes returns supported file extensions
	SupportedTypes() []string

	// MaxSize returns maximum supported file size
	MaxSize() int64
}

// CommandExecutor handles safe command execution
type CommandExecutor struct {
	history []CommandHistory
	sysInfo SystemInfo
}

// NewCommandExecutor creates a new executor with system info
func NewCommandExecutor() (*CommandExecutor, error) {
	sysInfo, err := GetSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %v", err)
	}

	return &CommandExecutor{
		history: make([]CommandHistory, 0),
		sysInfo: sysInfo,
	}, nil
}

// Execute runs a command safely
func (e *CommandExecutor) Execute(ctx context.Context, command string) (CommandHistory, error) {
	history := CommandHistory{
		Command:   command,
		StartTime: time.Now(),
	}

	// Validate command
	if err := e.ValidateCommand(command); err != nil {
		history.Error = err.Error()
		history.EndTime = time.Now()
		e.history = append(e.history, history)
		return history, err
	}

	// Prepare command based on platform
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	default:
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}

	// Set working directory
	cmd.Dir = e.sysInfo.WorkingDir

	// Execute command
	output, err := cmd.CombinedOutput()
	history.EndTime = time.Now()
	history.Output = string(output)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			history.ExitCode = exitErr.ExitCode()
		}
		history.Error = err.Error()
	}

	e.history = append(e.history, history)
	return history, err
}

// ValidateCommand checks if a command is safe to execute
func (e *CommandExecutor) ValidateCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return ErrValidationf("empty command")
	}

	// Check for dangerous commands
	dangerousCommands := []string{
		"rm -rf", "rmdir /s", "del /f",
		"format", "mkfs",
		":(){:|:&};:", // Fork bomb
		"dd",
		"> /dev/sda",
		"chmod -R 777",
	}

	for _, dangerous := range dangerousCommands {
		if strings.Contains(strings.ToLower(command), strings.ToLower(dangerous)) {
			return ErrPermissionf("potentially dangerous command detected: %s", dangerous)
		}
	}

	// Check if command requires privileges
	if IsPrivilegedOperation(command) {
		if !strings.HasPrefix(command, "sudo ") && runtime.GOOS != "windows" {
			return ErrPermissionf("this command requires elevated privileges")
		}
	}

	return nil
}

// GetHistory returns command execution history
func (e *CommandExecutor) GetHistory() []CommandHistory {
	return e.history
}

// ClearHistory clears the command history
func (e *CommandExecutor) ClearHistory() {
	e.history = make([]CommandHistory, 0)
}

// CommandHistory tracks command execution
type CommandHistory struct {
	Command   string    `json:"command"`
	Output    string    `json:"output"`
	ExitCode  int       `json:"exit_code"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Error     string    `json:"error,omitempty"`
}

// RateLimiter handles API rate limiting
type RateLimiter interface {
	// CheckLimit checks if operation is within limits
	CheckLimit() error

	// TrackTokens updates token usage
	TrackTokens(count int) error

	// GetUsage returns current usage stats
	GetUsage() map[string]interface{}

	// Reset resets all counters
	Reset()
}

// SystemInfo contains system information
type SystemInfo struct {
	OS          string            `json:"os"`
	Shell       string            `json:"shell"`
	User        string            `json:"user"`
	HomeDir     string            `json:"home_dir"`
	WorkingDir  string            `json:"working_dir"`
	Environment map[string]string `json:"environment"`
}
