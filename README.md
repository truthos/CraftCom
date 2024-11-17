# CraftCom
<img src="imgs/logo_nobg.png" height="240" width="240">

CraftCom is a command-line tool powered by Google's Gemini AI that helps users interact with their terminal through natural language. It converts natural language queries into shell commands, provides explanations, and ensures safe execution.

## Features

- 🤖 Natural language to shell command conversion
- 🛡️ Built-in safety checks and protections
- 🔍 Command explanation and validation
- 📁 File analysis capabilities
- 🎨 Colorful terminal output
- 🔧 Configurable settings
- 🌍 Multiple model support (Gemini-1.5-Pro, Gemini-1.5-Flash)

## Prerequisites

- Go 1.20 or higher
- Google Cloud Project with Gemini API enabled
- Gemini API key from [Google AI Studio](https://ai.google.dev/)

## Installation

### From Source

1. Clone the repository:
```bash
git clone https://github.com/truthos/CraftCom.git
cd craftcom
```

2. Build the project:
```bash
# Build for current platform
go build -o craftcom cmd/craftcom/main.go

# Or install directly to $GOPATH/bin
go install ./cmd/craftcom
```

### Using Go Install

```bash
go install github.com/truthos/CraftCom/cmd/craftcom@latest
```

## Configuration

1. Create a configuration file at `~/.craftcom.json`:
```json
{
  "providers": {
    "gemini": {
      "api_key": "YOUR_GEMINI_API_KEY",
      "enabled": true,
      "models": ["gemini-1.5-pro", "gemini-1.5-flash"]
    }
  },
  "default_provider": "gemini",
  "default_model": "gemini-1.5-pro",
  "safety_level": "medium"
}
```

2. Set your Gemini API key in the config file.

## Usage

```bash
# Basic usage
craftcom "list all pdf files"
craftcom "show system information"

# Use specific config file
craftcom -config /path/to/config.json "your command"

# Show version
craftcom -version
```

### Example Commands

```bash
# File operations
craftcom "find large files in current directory"
craftcom "create a backup of config.json"

# System information
craftcom "show disk usage"
craftcom "list running processes"

# File analysis
craftcom "analyze the contents of go.mod"
```

## Safety Features

- Command validation before execution
- Protected system directories
- Disallowed dangerous commands
- Configurable safety levels
- Clear command previews before execution

## Development

### Project Structure

```
craftcom/
├── cmd/                       # Command-line applications
│   └── craftcom/             # Main CLI application
├── docs/                     # Documentation
├── examples/                 # Example configurations and usage
├── pkg/                      # Public library code
│   ├── craftcom/            # Core library package
│   ├── gemini/              # Gemini provider implementation
│   └── types/               # Common types and interfaces
```

### Building From Source

```bash
# Get dependencies
go mod download

# Run tests
go test ./...

# Build
go build -o craftcom cmd/craftcom/main.go

# Install locally
go install ./cmd/craftcom
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./pkg/craftcom/...
```

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](docs/CONTRIBUTING.md) for details.

### Development Setup

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Roadmap

- [ ] Add support for more AI providers
- [ ] Implement command history and suggestions
- [ ] Add interactive mode
- [ ] Support for custom command templates
- [ ] Multi-language support

## Authors

- TruthOS - [github.com/truthos](github.com/truthos)

## Acknowledgments

- Google Gemini Team for the AI capabilities
- The Go community for the amazing tooling
- Contributors and users of the project