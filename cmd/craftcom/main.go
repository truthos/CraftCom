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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/atotto/clipboard"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"

	"craftcom/pkg/craftcom"
	"craftcom/pkg/gemini"
	"craftcom/pkg/types"
)

var (
	version = "0.1.0"
	bold    = color.New(color.Bold)
	info    = color.New(color.FgCyan)
	success = color.New(color.FgGreen)
	warning = color.New(color.FgYellow)
	errLog  = color.New(color.FgRed)
)

// Config Configuration structure
type Config struct {
	APIKey          string            `json:"api_key"`
	DefaultModel    string            `json:"default_model"`
	DefaultProvider string            `json:"default_provider"`
	HistorySize     int               `json:"history_size"`
	MaxTokens       int               `json:"max_tokens"`
	Temperature     float32           `json:"temperature"`
	SafetyLevel     string            `json:"safety_level"`
	Aliases         map[string]string `json:"aliases"`
	OutputFormat    string            `json:"output_format"`
	Debug           bool              `json:"debug"` // Add this field
}

// CLI flags and commands
// CLI flags and commands
type CLI struct {
	Config     string `help:"Configure file path" type:"path" short:"c"`
	Provider   string `help:"AI provider to use (default: gemini)" default:"gemini" short:"p"`
	Model      string `help:"Model to use" short:"m"`
	OutputFile string `help:"Output file for commands" type:"path" short:"o"`
	ReadmeFile string `help:"File for full markdown output" type:"path" short:"w"`
	Quiet      bool   `help:"Non-interactive mode" default:"false" short:"q"`
	Debug      bool   `help:"Enable debug mode" default:"false" short:"d"`
	Version    bool   `help:"Show version information" short:"v"`

	// Commands
	Execute   ExecuteCmd   `cmd:"" help:"Execute a specific natural language command" hidden:""`
	List      ListCmd      `cmd:"" help:"List available models"`
	History   HistoryCmd   `cmd:"" help:"Show command history"`
	Clear     ClearCmd     `cmd:"" help:"Clear history"`
	Configure ConfigureCmd `cmd:"" help:"Configure settings"`
}

type ExecuteCmd struct {
	Command string   `arg:"" optional:"" help:"Natural language command to execute"`
	Files   []string `arg:"" optional:"" help:"Files to process (images, PDFs, etc.)"`
}

type ListCmd struct{}

type HistoryCmd struct {
	Limit int  `help:"Number of entries to show" default:"10"`
	Full  bool `help:"Show full command details" default:"false" short:"l"`
}

type ClearCmd struct {
	Force bool `help:"Force clear without confirmation" short:"y"`
}

type ConfigureCmd struct {
	Reset bool `help:"Reset configuration to defaults" short:"x"`
}

// Application represents the main application state
type Application struct {
	config    Config
	assistant *libterma.Terma
	provider  types.Provider
	spinner   *spinner.Spinner
	kongCtx   *kong.Context // Add this field
}

func main() {
	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		cancel()
	}()

	var cli CLI
	kongCtx := kong.Parse(&cli,
		kong.Name("craftcom"),
		kong.Description("AI-powered terminal assistant"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}),
	)

	if cli.Version {
		fmt.Printf("craftcom version %s\n", version)
		os.Exit(0)
	}

	// Initialize application
	app, err := initializeApplication(&cli)
	if err != nil {
		errLog.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer app.cleanup()

	// If no command is specified, or if it's an execute command with no arguments,
	// start interactive mode
	if kongCtx.Command() == "" ||
		(kongCtx.Command() == "execute" && cli.Execute.Command == "") {
		if err := app.runInteractiveMode(ctx, &cli); err != nil {
			errLog.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise execute the selected command
	if err := app.run(ctx, kongCtx, &cli); err != nil {
		errLog.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func loadConfig(configPath string) (Config, error) {
	var config Config

	// Set defaults
	config = Config{
		DefaultProvider: "gemini",
		DefaultModel:    "gemini-1.5-pro",
		HistorySize:     1000,
		MaxTokens:       2048,
		Temperature:     0.7,
		SafetyLevel:     "medium",
		OutputFormat:    "markdown",
		Aliases:         make(map[string]string),
	}

	// If no config path specified, use default location
	if configPath == "" {
		home, err := os.UserHomeDir()

		if err != nil {
			return Config{}, fmt.Errorf("failed to get home directory: %v", err)
		}
		configPath = filepath.Join(home, ".craftcom.json")
	}

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return Config{}, fmt.Errorf("failed to create config directory: %v", err)
	}

	// Try to read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default config file
			data, err := json.MarshalIndent(config, "", "  ")
			if err != nil {
				return Config{}, fmt.Errorf("failed to marshal default config: %v", err)
			}

			if err := os.WriteFile(configPath, data, 0644); err != nil {
				return Config{}, fmt.Errorf("failed to write default config: %v", err)
			}

			info.Printf("Created default config at: %s\n", configPath)
			return config, nil
		}
		return Config{}, fmt.Errorf("failed to read config: %v", err)
	}

	// Parse existing config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("failed to parse config: %v", err)
	}

	return config, nil
}

func initializeApplication(cli *CLI) (*Application, error) {
	// Create spinner
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Prefix = "Initializing "
	s.Start()
	defer s.Stop()

	// Determine config path
	configPath := cli.Config
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %v", err)
		}
		configPath = filepath.Join(home, ".craftcom.json")
	}

	// Load configuration
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %v", err)
	}

	// Check for API key in environment if not in config
	if config.APIKey == "" {
		config.APIKey = os.Getenv("GEMINI_API_KEY")
	}

	if config.APIKey == "" {
		return nil, fmt.Errorf("API key not found in config or environment. Please set GEMINI_API_KEY or configure the API key")
	}

	// Initialize provider
	provider, err := initializeProvider(config, cli)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize provider: %v", err)
	}

	// Initialize assistant with the correct config path
	assistant, err := libterma.New(configPath) // Pass the actual config path here
	if err != nil {
		return nil, fmt.Errorf("failed to initialize assistant: %v", err)
	}

	return &Application{
		config:    config,
		assistant: assistant,
		provider:  provider,
		spinner:   s,
	}, nil
}

func initializeProvider(config Config, cli *CLI) (types.Provider, error) {
	// Get API key from config or environment
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("API key not found in config or environment")
	}

	// Create provider with system prompt
	provider, err := gemini.NewProvider(
		context.Background(),
		apiKey,
		createSystemPrompt(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %v", err)
	}

	return provider, nil
}

func createSystemPrompt() string {
	sysInfo, err := types.GetSystemInfo()
	if err != nil {
		return ""
	}

	return fmt.Sprintf(`You are a terminal command assistant for %s using %s shell.
Your goal is to help users by converting their natural language requests into appropriate
terminal commands. Always prioritize safety and provide clear explanations.`,
		sysInfo.OS,
		sysInfo.Shell,
	)
}

func (app *Application) run(ctx context.Context, kongCtx *kong.Context, cli *CLI) error {
	if cli.Debug {
		info.Println("Debug mode enabled")
	}

	switch kongCtx.Command() {
	case "list":
		return app.handleList(ctx)
	case "execute":
		return app.handleExecute(ctx, cli)
	case "history":
		return app.handleHistory(cli.History.Limit, cli.History.Full)
	case "clear":
		return app.handleClear(cli.Clear.Force)
	case "configure":
		return app.handleConfigure(cli.Configure.Reset)
	default:
		return app.runInteractiveMode(ctx, cli)
	}
}

func (app *Application) handleList(ctx context.Context) error {
	models, err := app.provider.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list models: %v", err)
	}

	info.Println("Available models:")
	for _, model := range models {
		modelInfo, err := app.provider.GetModelInfo(model)
		if err != nil {
			continue
		}
		fmt.Printf("- %s\n", model)
		fmt.Printf("  ├─ Input tokens: %d\n", modelInfo.InputTokenLimit)
		fmt.Printf("  ├─ Output tokens: %d\n", modelInfo.OutputTokenLimit)
		fmt.Printf("  ├─ Requests/min: %d\n", modelInfo.RPM)
		fmt.Printf("  └─ Features: %s\n", strings.Join(modelInfo.Features, ", "))
	}

	return nil
}

func (app *Application) handleExecute(ctx context.Context, cli *CLI) error {
	chat, err := app.assistant.Chat(ctx)
	if err != nil {
		return fmt.Errorf("failed to create chat: %v", err)
	}
	defer chat.Close()

	app.spinner.Prefix = "Processing "
	app.spinner.Start()

	var resp types.Response
	if len(cli.Execute.Files) > 0 {
		resp, err = chat.SendWithFiles(ctx, cli.Execute.Command, cli.Execute.Files)
	} else {
		resp, err = chat.Send(ctx, cli.Execute.Command)
	}
	app.spinner.Stop()

	if err != nil {
		return fmt.Errorf("failed to process command: %v", err)
	}

	// Display results
	bold.Println("\nGenerated Command:")
	fmt.Printf("$ %s\n\n", resp.Code)

	info.Println("Explanation:")
	fmt.Println(resp.FullOutput)

	if !cli.Quiet {
		if err := app.confirmAndExecute(ctx, resp.Code); err != nil {
			return err
		}
	}

	return nil
}

func (app *Application) confirmAndExecute(ctx context.Context, command string) error {
	confirm := promptui.Prompt{
		Label:     "Execute this command",
		IsConfirm: true,
	}

	result, err := confirm.Run()
	if err != nil || strings.ToLower(result) != "y" {
		return nil
	}

	app.spinner.Start()
	defer app.spinner.Stop()

	executor, err := types.NewCommandExecutor()
	if err != nil {
		return fmt.Errorf("failed to create command executor: %v", err)
	}

	cmdResult, err := executor.Execute(ctx, command)
	if err != nil {
		return fmt.Errorf("failed to execute command: %v", err)
	}

	success.Println("\nOutput:")
	fmt.Println(cmdResult.Output)

	return nil
}

func (app *Application) handleHistory(limit int, full bool) error {
	history := app.assistant.GetHistory()

	if len(history) == 0 {
		info.Println("No command history available")
		return nil
	}

	start := len(history) - limit
	if start < 0 {
		start = 0
	}

	info.Println("Command History:")
	for i, cmd := range history[start:] {
		bold.Printf("\n%d. Command:\n", i+1)
		fmt.Printf("$ %s\n", cmd.Command)
		if full {
			fmt.Printf("Time: %s\n", cmd.StartTime.Format(time.RFC3339))
			fmt.Printf("Exit Code: %d\n", cmd.ExitCode)
			if cmd.Error != "" {
				fmt.Printf("Error: %s\n", cmd.Error)
			}
		}
		fmt.Println("Output:")
		fmt.Println(cmd.Output)
	}

	return nil
}

func (app *Application) handleClear(force bool) error {
	if !force {
		confirm := promptui.Prompt{
			Label:     "Clear command history",
			IsConfirm: true,
		}

		result, err := confirm.Run()
		if err != nil || strings.ToLower(result) != "y" {
			return nil
		}
	}

	app.assistant.ClearHistory()
	success.Println("Command history cleared")
	return nil
}

func (app *Application) handleConfigure(reset bool) error {
	if reset {
		// Reset configuration to defaults
		config, err := loadConfig("")
		if err != nil {
			return fmt.Errorf("failed to load default configuration: %v", err)
		}
		app.config = config
		success.Println("Configuration reset to defaults")
		return nil
	}

	// Interactive configuration
	return app.runConfigurationWizard()
}

func (app *Application) runConfigurationWizard() error {
	// Implementation of interactive configuration wizard
	// This would allow users to set various configuration options
	return nil
}

func (app *Application) runInteractiveMode(ctx context.Context, cli *CLI) error {
	chat, err := app.assistant.Chat(ctx)
	if err != nil {
		return fmt.Errorf("failed to create chat: %v", err)
	}
	defer chat.Close()

	app.displayWelcomeMessage()

	for {
		prompt := promptui.Prompt{
			Label: ">",
			Templates: &promptui.PromptTemplates{
				Prompt:  "{{ . | cyan }}▶ ",
				Valid:   "{{ . | green }}▶ ",
				Invalid: "{{ . | red }}▶ ",
			},
		}

		input, err := prompt.Run()
		if err != nil {
			return fmt.Errorf("prompt error: %v", err)
		}

		if input == "exit" || input == "quit" {
			break
		}

		if err := app.handleInteractiveCommand(ctx, chat, input); err != nil {
			errLog.Printf("Error: %v\n", err)
		}
	}

	return nil
}

func (app *Application) handleInteractiveCommand(ctx context.Context, chat types.Chat, input string) error {
	// Check if the input mentions a file
	fileRegex := regexp.MustCompile(`(?i)(analyze|read|describe|show|check|look at|view|process)\s+.*?(file|image|photo|picture|document|pdf)\s+([^\s]+)`)
	if match := fileRegex.FindStringSubmatch(input); len(match) > 3 {
		filePath := match[3]
		// Clean up the path (remove quotes if present)
		filePath = strings.Trim(filePath, `"'`)

		return app.handleFileAnalysis(ctx, chat, filePath, input)
	}

	app.spinner.Prefix = "Thinking "
	app.spinner.Start()

	resp, err := chat.Send(ctx, input)
	app.spinner.Stop()

	if err != nil {
		return err
	}

	// Extract all possible commands from the response
	commands := extractCommands(resp.FullOutput)

	if len(commands) == 0 {
		// This is a response without commands - format it nicely
		app.displayInformationalResponse(resp.FullOutput)
		return nil
	}

	// Rest of the command handling remains the same...
	return app.handleCommandSuggestions(ctx, commands, resp)
}

func (app *Application) handleFileAnalysis(ctx context.Context, chat types.Chat, filePath string, originalInput string) error {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		errLog.Printf("File not found: %s\n", filePath)
		return nil
	}

	app.spinner.Prefix = "Analyzing file "
	app.spinner.Start()

	// Send the request with the file
	resp, err := chat.SendWithFiles(ctx, originalInput, []string{filePath})
	app.spinner.Stop()

	if err != nil {
		errLog.Printf("Error analyzing file: %v\n", err)
		return nil
	}

	// Display the analysis results
	info.Println("\nFile Analysis Results:")
	fmt.Println(resp.FullOutput)

	// Check if there are any commands in the response
	commands := extractCommands(resp.FullOutput)
	if len(commands) > 0 {
		return app.handleCommandSuggestions(ctx, commands, resp)
	}

	return nil
}

func (app *Application) displayInformationalResponse(output string) {
	sections := strings.Split(output, "\n\n")

	for _, section := range sections {
		if strings.TrimSpace(section) == "" {
			continue
		}

		// Check if this is a bullet point list
		if strings.Contains(section, "\n* ") || strings.Contains(section, "\n- ") {
			info.Println("\nSuggestions:")
			fmt.Println(section)
		} else {
			// Regular paragraph
			fmt.Printf("\n%s\n", section)
		}
	}

	// If the response mentioned file handling, add helpful tip
	if strings.Contains(strings.ToLower(output), "file") ||
		strings.Contains(strings.ToLower(output), "image") {
		tip := color.New(color.FgYellow).SprintFunc()
		fmt.Printf("\n%s\n", tip("Tip: I can analyze images and some document types directly. "+
			"Just make sure the file path is correct and the file is accessible."))
	}
}

func (app *Application) handleCommandSuggestions(ctx context.Context, commands []string, resp types.Response) error {
	if len(commands) == 1 {
		bold.Println("\nSuggested Command:")
		fmt.Printf("$ %s\n\n", commands[0])
		fmt.Println(resp.FullOutput)

		return app.handleCommandExecution(ctx, commands[0], resp)
	}

	bold.Println("\nMultiple commands suggested:")
	for i, cmd := range commands {
		fmt.Printf("%d. $ %s\n", i+1, cmd)
	}
	fmt.Println("\nExplanation:")
	fmt.Println(resp.FullOutput)

	prompt := promptui.Select{
		Label: "Which command would you like to execute?",
		Items: append(commands, "Skip"),
	}

	_, result, err := prompt.Run()
	if err != nil {
		return fmt.Errorf("prompt error: %v", err)
	}

	if result == "Skip" {
		return nil
	}

	return app.handleCommandExecution(ctx, result, resp)
}

func extractCommands(output string) []string {
	var commands []string

	// Extract commands from code blocks
	codeBlockRegex := regexp.MustCompile("```(?:bash|shell|zsh|cmd|powershell)?\n(.*?)\n```")
	matches := codeBlockRegex.FindAllStringSubmatch(output, -1)
	for _, match := range matches {
		if len(match) > 1 {
			cmd := strings.TrimSpace(match[1])
			if cmd != "" {
				commands = append(commands, cmd)
			}
		}
	}

	// Extract commands from lines starting with $
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$ ") {
			cmd := strings.TrimSpace(strings.TrimPrefix(line, "$ "))
			if cmd != "" && !contains(commands, cmd) {
				commands = append(commands, cmd)
			}
		}
	}

	return commands
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (app *Application) handleCommandExecution(ctx context.Context, command string, resp types.Response) error {
	options := []string{
		"Execute",
		"Copy to clipboard",
		"Show explanation",
		"Show command details",
		"Skip",
	}

	prompt := promptui.Select{
		Label: "What would you like to do?",
		Items: options,
		Size:  len(options),
	}

	_, result, err := prompt.Run()
	if err != nil {
		return fmt.Errorf("prompt error: %v", err)
	}

	switch result {
	case "Execute":
		return app.confirmAndExecute(ctx, command)
	case "Copy to clipboard":
		if err := clipboard.WriteAll(command); err != nil {
			return fmt.Errorf("failed to copy to clipboard: %v", err)
		}
		success.Println("Command copied to clipboard")
	case "Show explanation":
		info.Println("\nDetailed Explanation:")
		fmt.Println(resp.FullOutput)
	case "Show command details":
		app.displayCommandDetails(resp)
	}

	return nil
}

func (app *Application) displayCommandDetails(resp types.Response) {
	info.Println("\nCommand Details:")
	fmt.Printf("Model: %s\n", resp.Metadata["model"])
	fmt.Printf("Tokens Used: %d\n", resp.Metadata["tokens_used"])
	fmt.Printf("Command Count: %d\n", resp.Metadata["command_count"])
	fmt.Printf("Error Count: %d\n", resp.Metadata["error_count"])
	fmt.Printf("Session Length: %.2f minutes\n", resp.Metadata["session_length"])
}

func (app *Application) displayWelcomeMessage() {
	asciiArt := `
    ╔═══════════════════════════════════════╗
    ║            CraftCom v%s               ║
    ║  AI-Powered Terminal Assistant        ║
    ╚═══════════════════════════════════════╝

    Type 'help' for commands, 'exit' to quit
    Available commands:
    - execute <command>   : Execute a specific command
    - history [n]        : Show last n commands
    - clear             : Clear command history
    - list              : List available models
    - configure         : Configure settings
    `
	info.Printf(asciiArt, version)
	fmt.Println()
}

func (app *Application) cleanup() {
	if app.assistant != nil {
		app.assistant.Close()
	}
	if app.provider != nil {
		app.provider.Close()
	}
}

func (app *Application) saveOutput(resp types.Response, cli *CLI) error {
	// Save command output if requested
	if cli.OutputFile != "" {
		if err := os.WriteFile(cli.OutputFile, []byte(resp.Code), 0644); err != nil {
			return fmt.Errorf("failed to save command output: %v", err)
		}
		success.Printf("Command saved to: %s\n", cli.OutputFile)
	}

	// Save full markdown output if requested
	if cli.ReadmeFile != "" {
		markdown := fmt.Sprintf("# Command Documentation\n\n## Command\n```bash\n%s\n```\n\n## Explanation\n%s\n",
			resp.Code, resp.FullOutput)
		if err := os.WriteFile(cli.ReadmeFile, []byte(markdown), 0644); err != nil {
			return fmt.Errorf("failed to save markdown output: %v", err)
		}
		success.Printf("Documentation saved to: %s\n", cli.ReadmeFile)
	}

	return nil
}

// handleHelp displays available commands and their usage
func (app *Application) handleHelp() {
	help := `
Available Commands:
    execute <command>  Execute a specific command
    history [n]        Show last n commands (default: 10)
    clear             Clear command history
    list              List available models
    configure         Configure settings
    help              Show this help message
    exit/quit         Exit the application

Special Commands:
    !<n>              Re-run command number n from history
    !!                Re-run last command
    !$                Use last command's arguments
    !*                Use all arguments from last command

Options:
    -q, --quiet       Non-interactive mode
    -d, --debug       Enable debug mode
    -o, --output      Save command to file
    -r, --readme      Save full documentation to file
    `
	fmt.Println(help)
}

// handleSpecialCommand processes special command syntax (e.g., !!, !$, etc.)
func (app *Application) handleSpecialCommand(input string, history []types.CommandHistory) (string, error) {
	if len(history) == 0 {
		return "", fmt.Errorf("no command history available")
	}

	switch input {
	case "!!":
		return history[len(history)-1].Command, nil
	case "!$":
		parts := strings.Fields(history[len(history)-1].Command)
		if len(parts) > 1 {
			return parts[len(parts)-1], nil
		}
		return "", fmt.Errorf("no arguments in last command")
	case "!*":
		parts := strings.Fields(history[len(history)-1].Command)
		if len(parts) > 1 {
			return strings.Join(parts[1:], " "), nil
		}
		return "", fmt.Errorf("no arguments in last command")
	}

	// Handle !n syntax
	if strings.HasPrefix(input, "!") {
		num := strings.TrimPrefix(input, "!")
		index, err := strconv.Atoi(num)
		if err != nil {
			return "", fmt.Errorf("invalid history reference: %s", input)
		}
		if index < 1 || index > len(history) {
			return "", fmt.Errorf("history index out of range: %d", index)
		}
		return history[index-1].Command, nil
	}

	return input, nil
}

// Debug logging helper
func (app *Application) debug(format string, args ...interface{}) {
	if app.config.Debug {
		info.Printf("[DEBUG] "+format+"\n", args...)
	}
}
