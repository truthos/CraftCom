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
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// GetSystemInfo returns detailed system information
func GetSystemInfo() (SystemInfo, error) {
	info := SystemInfo{
		OS:          runtime.GOOS,
		Environment: make(map[string]string),
	}

	// Get user and home directory
	currentUser, err := user.Current()
	if err != nil {
		return info, ErrSystemf("failed to get username: %v", err)
	}
	info.User = currentUser.Username

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return info, ErrSystemf("failed to get home directory: %v", err)
	}
	info.HomeDir = homeDir

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return info, ErrSystemf("failed to get working directory: %v", err)
	}
	info.WorkingDir = workDir

	// Detect shell
	info.Shell = detectShell()

	// Get relevant environment variables
	relevantVars := []string{
		"PATH", "SHELL", "TERM", "LANG", "LC_ALL",
		"HOME", "USER", "HOSTNAME", "EDITOR",
	}

	for _, v := range relevantVars {
		if val, exists := os.LookupEnv(v); exists {
			info.Environment[v] = val
		}
	}

	return info, nil
}

// detectShell determines the current shell being used
func detectShell() string {
	// First check SHELL environment variable
	if shell := os.Getenv("SHELL"); shell != "" {
		return filepath.Base(shell)
	}

	// Platform specific detection
	switch runtime.GOOS {
	case "windows":
		// Check if PowerShell is available
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			return "powershell"
		}
		return "cmd"

	default: // Unix-like systems
		// Try to detect from process
		if pid := os.Getppid(); pid != 0 {
			if bytes, err := os.ReadFile(filepath.Join("/proc", string(pid), "cmdline")); err == nil {
				cmdline := string(bytes)
				for _, shell := range []string{"bash", "zsh", "fish", "sh"} {
					if strings.Contains(cmdline, shell) {
						return shell
					}
				}
			}
		}
		// Default to bash if unable to detect
		return "bash"
	}
}

// IsPrivilegedOperation checks if a command requires elevated privileges
func IsPrivilegedOperation(command string) bool {
	command = strings.TrimSpace(command)

	// Check for sudo/su usage
	if strings.HasPrefix(command, "sudo ") || strings.HasPrefix(command, "su ") {
		return true
	}

	// Check for Windows admin commands
	if runtime.GOOS == "windows" {
		adminCommands := []string{
			"net user", "net localgroup",
			"reg add", "reg delete",
			"sc ", "bcdedit",
		}
		for _, cmd := range adminCommands {
			if strings.HasPrefix(strings.ToLower(command), strings.ToLower(cmd)) {
				return true
			}
		}
	}

	// Check for system-critical paths
	criticalPaths := []string{
		"/etc/", "/usr/", "/var/",
		"/bin/", "/sbin/",
		"C:\\Windows\\", "C:\\Program Files\\",
	}

	for _, path := range criticalPaths {
		if strings.Contains(command, path) {
			return true
		}
	}

	return false
}
