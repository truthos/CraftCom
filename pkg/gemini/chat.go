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

package gemini

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"craftcom/pkg/types"
	"github.com/google/generative-ai-go/genai"
)

// Chat represents a chat session with the Gemini model
type Chat struct {
	model          *genai.GenerativeModel
	chat           *genai.ChatSession
	modelConfig    ModelConfig
	rateLimiter    *RateLimiter
	history        []genai.Content
	safetySettings []*genai.SafetySetting
	fileProcessor  *types.FileReader
	currentContext *ChatContext
}

// ChatContext maintains the current conversation context
type ChatContext struct {
	WorkingDir   string
	LastCommand  string
	LastOutput   string
	Environment  map[string]string
	SystemInfo   types.SystemInfo
	LastModified time.Time
	SessionStart time.Time
	CommandCount int
	ErrorCount   int
}

// NewChatContext creates a new chat context with system information
func NewChatContext() (*ChatContext, error) {
	sysInfo, err := types.GetSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %v", err)
	}

	return &ChatContext{
		WorkingDir:   sysInfo.WorkingDir,
		Environment:  make(map[string]string),
		SystemInfo:   sysInfo,
		SessionStart: time.Now(),
		LastModified: time.Now(),
		CommandCount: 0,
		ErrorCount:   0,
	}, nil
}

// Send sends a message to the chat
func (c *Chat) Send(ctx context.Context, message string) (types.Response, error) {
	if err := c.rateLimiter.CheckLimit(); err != nil {
		return types.Response{}, err
	}

	// Add context to message
	contextualMessage := c.addContext(message)

	// Create prompt parts
	parts := []genai.Part{
		genai.Text(contextualMessage),
	}

	// Generate content instead of using SendMessage
	resp, err := c.model.GenerateContent(ctx, parts...)
	if err != nil {
		c.currentContext.ErrorCount++
		return types.Response{}, types.ErrExecutionf("failed to generate content: %v", err)
	}

	if len(resp.Candidates) == 0 {
		c.currentContext.ErrorCount++
		return types.Response{}, types.ErrExecutionf("no response generated")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		c.currentContext.ErrorCount++
		return types.Response{}, types.ErrExecutionf("empty response content")
	}

	// Extract command and output
	command, fullOutput := c.extractCommandAndOutput(*candidate.Content)

	// Update history
	c.history = append(c.history,
		genai.Content{Parts: []genai.Part{genai.Text(contextualMessage)}, Role: "user"},
		*candidate.Content,
	)

	// Update context
	c.currentContext.LastCommand = command
	c.currentContext.LastModified = time.Now()
	c.currentContext.CommandCount++

	// Estimate token usage
	tokenCount := estimateTokenCount(contextualMessage + fullOutput)
	if err := c.rateLimiter.TrackTokens(tokenCount); err != nil {
		return types.Response{}, err
	}

	return types.Response{
		Code:       command,
		FullOutput: fullOutput,
		Metadata: map[string]interface{}{
			"model":          c.modelConfig.Name,
			"timestamp":      time.Now(),
			"context":        c.currentContext,
			"tokens_used":    tokenCount,
			"command_count":  c.currentContext.CommandCount,
			"error_count":    c.currentContext.ErrorCount,
			"session_length": time.Since(c.currentContext.SessionStart).Minutes(),
		},
	}, nil
}

func (c *Chat) initializeChat(ctx context.Context, systemPrompt string) error {
	// Create new chat context
	chatContext, err := NewChatContext()
	if err != nil {
		return types.ErrConfigurationf("failed to create chat context: %v", err)
	}
	c.currentContext = chatContext

	// Initialize history with system prompt
	if systemPrompt != "" {
		c.history = []genai.Content{
			{
				Parts: []genai.Part{genai.Text(systemPrompt)},
				Role:  "system",
			},
		}
	}

	return nil
}

// SendWithFiles sends a message with file attachments
func (c *Chat) SendWithFiles(ctx context.Context, message string, files []string) (types.Response, error) {
	if err := c.rateLimiter.CheckLimit(); err != nil {
		return types.Response{}, err
	}

	var parts []genai.Part
	parts = append(parts, genai.Text(c.addContext(message)))

	// Process files
	processedFiles := make([]string, 0, len(files))
	totalSize := int64(0)

	for _, file := range files {
		// Check file exists
		if _, err := os.Stat(file); err != nil {
			return types.Response{}, types.ErrInputf("file not found: %s", file)
		}

		// Process file
		content, err := c.fileProcessor.ReadFile(ctx, file)
		if err != nil {
			return types.Response{}, types.ErrInputf("failed to process file %s: %v", file, err)
		}

		// Check total size
		totalSize += content.Size
		if totalSize > c.fileProcessor.MaxSize {
			return types.Response{}, types.ErrInputf("total file size exceeds limit")
		}

		// Create appropriate part based on file type
		part, err := c.createPartFromContent(content)
		if err != nil {
			return types.Response{}, err
		}

		if part != nil {
			parts = append(parts, part)
			processedFiles = append(processedFiles, file)
		}
	}

	// Generate response with files
	resp, err := c.model.GenerateContent(ctx, parts...)
	if err != nil {
		c.currentContext.ErrorCount++
		return types.Response{}, types.ErrExecutionf("failed to generate content: %v", err)
	}

	if len(resp.Candidates) == 0 {
		c.currentContext.ErrorCount++
		return types.Response{}, types.ErrExecutionf("no response generated")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil {
		c.currentContext.ErrorCount++
		return types.Response{}, types.ErrExecutionf("empty response content")
	}

	command, fullOutput := c.extractCommandAndOutput(*candidate.Content)

	// Update context
	c.currentContext.LastCommand = command
	c.currentContext.LastModified = time.Now()
	c.currentContext.CommandCount++

	// Track token usage
	tokenCount := estimateTokenCount(message + fullOutput)
	if err := c.rateLimiter.TrackTokens(tokenCount); err != nil {
		return types.Response{}, err
	}

	return types.Response{
		Code:       command,
		FullOutput: fullOutput,
		Metadata: map[string]interface{}{
			"model":          c.modelConfig.Name,
			"timestamp":      time.Now(),
			"files":          processedFiles,
			"context":        c.currentContext,
			"tokens_used":    tokenCount,
			"command_count":  c.currentContext.CommandCount,
			"error_count":    c.currentContext.ErrorCount,
			"session_length": time.Since(c.currentContext.SessionStart).Minutes(),
		},
	}, nil
}

// Helper functions

func (c *Chat) addContext(message string) string {
	return fmt.Sprintf(`Current directory: %s
Last command: %s
Last output: %s
OS: %s
Shell: %s

User request: %s`,
		c.currentContext.WorkingDir,
		c.currentContext.LastCommand,
		c.currentContext.LastOutput,
		c.currentContext.SystemInfo.OS,
		c.currentContext.SystemInfo.Shell,
		message,
	)
}

func (c *Chat) extractCommandAndOutput(content genai.Content) (string, string) {
	var command, fullOutput string

	for _, part := range content.Parts {
		if text, ok := part.(genai.Text); ok {
			fullOutput += string(text)
		}
	}

	// Try to extract command from markdown code blocks first
	codeBlockRegex := regexp.MustCompile("```(?:bash|shell|zsh|cmd|powershell)?\n(.*?)\n```")
	matches := codeBlockRegex.FindAllStringSubmatch(fullOutput, -1)
	if len(matches) > 0 && len(matches[0]) > 1 {
		// Get the content of the first code block
		command = strings.TrimSpace(matches[0][1])
		// Validate it looks like a command (not just a path or text)
		if isValidCommand(command) {
			return command, fullOutput
		}
	}

	// If no valid command found in code blocks, try inline code blocks
	inlineCodeRegex := regexp.MustCompile("`([^`]+)`")
	matches = inlineCodeRegex.FindAllStringSubmatch(fullOutput, -1)
	if len(matches) > 0 && len(matches[0]) > 1 {
		command = strings.TrimSpace(matches[0][1])
		if isValidCommand(command) {
			return command, fullOutput
		}
	}

	// Try to extract from common command indicators
	lines := strings.Split(fullOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$") ||
			strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, ">") {
			command = strings.TrimSpace(strings.TrimPrefix(
				strings.TrimPrefix(
					strings.TrimPrefix(line, "$"),
					"#"),
				">"))
			if isValidCommand(command) {
				return command, fullOutput
			}
		}
	}

	// If still no command found, look for specific keywords
	commandRegex := regexp.MustCompile(`(?i)(?:run|execute|command):\s*(.+)`)
	if matches := commandRegex.FindStringSubmatch(fullOutput); len(matches) > 1 {
		command = strings.TrimSpace(matches[1])
		if isValidCommand(command) {
			return command, fullOutput
		}
	}

	return "", fullOutput
}

// isValidCommand checks if a string looks like a valid command
func isValidCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}

	// Check if it's just a path (starts with / and contains no spaces or operators)
	if strings.HasPrefix(cmd, "/") && !strings.ContainsAny(cmd, " |&;<>()$`\\\"'") {
		return false
	}

	// Common command prefixes/words
	commonCmds := []string{
		"ls", "cd", "pwd", "cp", "mv", "rm", "mkdir", "touch", "cat",
		"echo", "grep", "find", "sed", "awk", "curl", "wget", "git",
		"docker", "make", "gcc", "go", "python", "node", "npm",
	}

	// Check if it starts with a common command
	for _, prefix := range commonCmds {
		if strings.HasPrefix(cmd, prefix+" ") || cmd == prefix {
			return true
		}
	}

	// Check for common operators that indicate it's a command
	if strings.ContainsAny(cmd, "|&;<>()") {
		return true
	}

	// Check for environment variables or subcommands
	if strings.Contains(cmd, "$") || strings.Contains(cmd, "`") {
		return true
	}

	return false
}

func (c *Chat) createPartFromContent(content *types.FileContent) (genai.Part, error) {
	switch content.Type {
	case types.FileTypeImage:
		return genai.ImageData(content.MimeType, content.Data), nil
	case types.FileTypeText:
		return genai.Text(content.String()), nil
	case types.FileTypePDF:
		text, err := extractTextFromPDF(content.Data)
		if err != nil {
			return nil, types.ErrInputf("failed to extract text from PDF: %v", err)
		}
		return genai.Text(text), nil
	default:
		return nil, types.ErrInputf("unsupported file type: %s", content.Type)
	}
}

// Utility functions

func extractCommandFromMarkdown(text string) string {
	codeBlockRegex := regexp.MustCompile("```(?:bash|shell|cmd|powershell)?\n(.*?)\n```")
	matches := codeBlockRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func extractCommandFromText(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for common command indicators
		if strings.HasPrefix(line, "$") ||
			strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, ">") ||
			strings.Contains(line, "run:") ||
			strings.Contains(line, "execute:") {
			return strings.TrimSpace(strings.TrimPrefix(
				strings.TrimPrefix(
					strings.TrimPrefix(
						strings.TrimPrefix(line, "$"),
						"#"),
					">"),
				"run:"))
		}
	}
	return ""
}

func estimateTokenCount(text string) int {
	words := len(strings.Fields(text))
	characters := len(text)

	wordBasedEstimate := float64(words) * 1.3
	charBasedEstimate := float64(characters) / 4.0

	return int((wordBasedEstimate + charBasedEstimate) / 2)
}

// extractTextFromPDF extracts text content from PDF data
func extractTextFromPDF(data []byte) (string, error) {
	// TODO: Implement PDF text extraction
	return "", types.ErrInputf("PDF processing not implemented")
}

// Close cleans up resources
func (c *Chat) Close() error {
	c.history = nil
	c.currentContext = nil
	return nil
}
