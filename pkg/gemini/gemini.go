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
	"sync"
	"time"

	"craftcom/pkg/types"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ModelConfig defines configuration for different Gemini models
type ModelConfig struct {
	Name             string
	InputTokenLimit  int
	OutputTokenLimit int
	RPM              int // Rate limits per minute
	TPM              int // Tokens per minute
	RPD              int // Requests per day
	IsPaid           bool
	MaxImages        int // Maximum number of images per prompt
	MaxVideoLength   int // Maximum video length in seconds
	MaxAudioLength   int // Maximum audio length in seconds
	Features         []string
	Temperature      float32 // Added temperature control
	TopK             int     // Added parameter for response diversity
	TopP             float32 // Added parameter for nucleus sampling
	MaxOutputTokens  int     // Maximum tokens in response
}

// Available Gemini models with optimized configurations
var (
	ModelGemini15Pro = ModelConfig{
		Name:             "gemini-1.5-pro",
		InputTokenLimit:  2_097_152,
		OutputTokenLimit: 8_192,
		RPM:              60,      //
		TPM:              450_000, //
		RPD:              1_000,   //
		IsPaid:           false,
		MaxImages:        16,
		MaxVideoLength:   7200,  // 2 hours
		MaxAudioLength:   68400, // ~19 hours
		Features: []string{
			"text",
			"images",
			"audio",
			"video",
			"system_instructions",
			"function_calling",
		},
		Temperature:     0.7,
		TopK:            40,
		TopP:            0.95,
		MaxOutputTokens: 8192,
	}

	ModelGemini15Flash = ModelConfig{
		Name:             "gemini-1.5-flash",
		InputTokenLimit:  1_048_576,
		OutputTokenLimit: 8_192,
		RPM:              120,
		TPM:              1_000_000,
		RPD:              10_000,
		IsPaid:           false,
		MaxImages:        16,
		MaxVideoLength:   3600,
		MaxAudioLength:   34200,
		Features: []string{
			"text",
			"images",
			"audio",
			"video",
			"system_instructions",
			"function_calling",
		},
		Temperature:     0.8,
		TopK:            20,
		TopP:            0.9,
		MaxOutputTokens: 4096,
	}
)

// Provider implements the Gemini AI provider
type Provider struct {
	client            *genai.Client
	models            map[string]ModelConfig
	rateLimiters      map[string]*RateLimiter
	systemInstruction string
	defaultModel      string
	mu                sync.RWMutex
}

// NewProvider creates a new Gemini provider instance
func NewProvider(ctx context.Context, apiKey string, systemInstruction string) (*Provider, error) {
	if apiKey == "" {
		return nil, types.ErrConfigurationf("Gemini API key is required")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, types.ErrConfigurationf("failed to create Gemini client: %v", err)
	}

	// Initialize provider with available models
	provider := &Provider{
		client: client,
		models: map[string]ModelConfig{
			ModelGemini15Pro.Name:   ModelGemini15Pro,
			ModelGemini15Flash.Name: ModelGemini15Flash,
		},
		rateLimiters:      make(map[string]*RateLimiter),
		systemInstruction: systemInstruction,
		defaultModel:      ModelGemini15Pro.Name,
	}

	// Initialize rate limiters for each model
	for name, config := range provider.models {
		provider.rateLimiters[name] = NewRateLimiter(config)
	}

	return provider, nil
}

// Chat creates a new chat session with specified model
func (p *Provider) Chat(ctx context.Context, model string) (types.Chat, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if model == "" {
		model = p.defaultModel
	}

	config, err := p.GetModelConfig(model)
	if err != nil {
		return nil, types.ErrModelf("failed to get model config: %v", err)
	}

	rateLimiter, exists := p.rateLimiters[model]
	if !exists {
		rateLimiter = NewRateLimiter(config)
		p.rateLimiters[model] = rateLimiter
	}

	// Create model instance with configuration
	genModel := p.client.GenerativeModel(model)
	genModel.SetTemperature(config.Temperature)
	genModel.SetTopK(int32(config.TopK))
	genModel.SetTopP(config.TopP)
	genModel.SetMaxOutputTokens(int32(config.MaxOutputTokens))

	// Configure safety settings
	genModel.SafetySettings = []*genai.SafetySetting{
		{
			Category:  genai.HarmCategoryDangerousContent,
			Threshold: genai.HarmBlockMediumAndAbove,
		},
		{
			Category:  genai.HarmCategoryHarassment,
			Threshold: genai.HarmBlockMediumAndAbove,
		},
		{
			Category:  genai.HarmCategoryHateSpeech,
			Threshold: genai.HarmBlockMediumAndAbove,
		},
		{
			Category:  genai.HarmCategorySexuallyExplicit,
			Threshold: genai.HarmBlockMediumAndAbove,
		},
	}

	// Create chat instance
	chat := &Chat{
		model:          genModel,
		modelConfig:    config,
		rateLimiter:    rateLimiter,
		fileProcessor:  types.NewFileReader(),
		safetySettings: genModel.SafetySettings,
	}

	// Initialize chat with system instruction
	if err := chat.initializeChat(ctx, p.systemInstruction); err != nil {
		return nil, fmt.Errorf("failed to initialize chat: %v", err)
	}

	return chat, nil
}

// GetModelInfo returns model configuration in the standard format
func (p *Provider) GetModelInfo(model string) (types.ModelInfo, error) {
	config, err := p.GetModelConfig(model)
	if err != nil {
		return types.ModelInfo{}, err
	}

	return types.ModelInfo{
		Name:             config.Name,
		InputTokenLimit:  config.InputTokenLimit,
		OutputTokenLimit: config.OutputTokenLimit,
		RPM:              config.RPM,
		TPM:              config.TPM,
		RPD:              config.RPD,
		Features:         config.Features,
		Timeout:          10 * time.Minute,
		IsPaid:           config.IsPaid,
	}, nil
}

// ListModels returns available Gemini models
func (p *Provider) ListModels(ctx context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	models := make([]string, 0, len(p.models))
	for name := range p.models {
		models = append(models, name)
	}
	return models, nil
}

// GetModelConfig returns configuration for a specific model
func (p *Provider) GetModelConfig(model string) (ModelConfig, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	config, ok := p.models[model]
	if !ok {
		return ModelConfig{}, types.ErrModelf("unknown model: %s", model)
	}
	return config, nil
}

// ValidateConfig checks if the provider is properly configured
func (p *Provider) ValidateConfig() error {
	if p.client == nil {
		return types.ErrConfigurationf("Gemini client not initialized")
	}

	// Test API key with a simple request
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model := p.client.GenerativeModel(p.defaultModel)
	resp, err := model.GenerateContent(ctx, genai.Text("test"))
	if err != nil {
		return types.ErrConfigurationf("invalid API key or configuration: %v", err)
	}

	if resp == nil || len(resp.Candidates) == 0 {
		return types.ErrConfigurationf("received empty response from API")
	}

	return nil
}

// SetSystemInstruction updates the system instruction
func (p *Provider) SetSystemInstruction(instruction string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.systemInstruction = instruction
}

// CreateSystemPrompt creates a system prompt for terminal commands
func createSystemPrompt() string {
	sysInfo, err := types.GetSystemInfo()
	if err != nil {
		return ""
	}

	return fmt.Sprintf(
		"You are a terminal command assistant for %s using %s shell.\n"+
			"Your goal is to help users by:\n"+
			"1. Converting natural language requests into terminal commands\n"+
			"2. Analyzing files and images when requested\n"+
			"3. Providing clear explanations and guidance\n\n"+
			"When responding with commands, ALWAYS use this format:\n\n"+
			"```%s\n"+
			"<command here>\n"+
			"```\n\n"+
			"When analyzing files:\n"+
			"1. For images: Describe what you see in detail\n"+
			"2. For text files: Summarize the content\n"+
			"3. For structured files: Explain the structure and content\n\n"+
			"Guidelines:\n"+
			"  - Always use appropriate code blocks for commands\n"+
			"  - Be precise and clear in command syntax\n"+
			"  - Include necessary error checks\n"+
			"  - Explain potential risks\n"+
			"  - When analyzing files, be descriptive and thorough\n\n"+
			"DO NOT:\n"+
			"  - Execute destructive commands\n"+
			"  - Modify system critical paths\n"+
			"  - Execute commands that could compromise security\n"+
			"  - Use sudo unless explicitly requested\n"+
			"  - Claim you cannot analyze files when they are provided\n",
		sysInfo.OS,
		sysInfo.Shell,
		sysInfo.Shell)
}

// Close cleans up the provider resources
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		err := p.client.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
