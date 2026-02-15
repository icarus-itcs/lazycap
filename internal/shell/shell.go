package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/icarus-itcs/lazycap/internal/settings"
)

// EssentialPaths contains standard system directories that must be in PATH
// for tools like tr, rsync, xcodebuild scripts, etc. to work.
var EssentialPaths = []string{
	"/usr/local/bin",
	"/usr/bin",
	"/bin",
	"/usr/sbin",
	"/sbin",
}

// ShellType represents the detected shell type.
type ShellType int

const (
	Unknown ShellType = iota
	Zsh
	Bash
	Fish
	Sh
)

// DetectShell determines the shell type from a binary path.
// Handles versioned names like bash-5.2 or zsh-5.9.
func DetectShell(path string) ShellType {
	base := filepath.Base(path)
	switch {
	case base == "zsh" || strings.HasPrefix(base, "zsh-"):
		return Zsh
	case base == "bash" || strings.HasPrefix(base, "bash-"):
		return Bash
	case base == "fish" || strings.HasPrefix(base, "fish-"):
		return Fish
	case base == "sh" || base == "dash":
		return Sh
	default:
		return Unknown
	}
}

// ResolveShell returns the shell binary path to use.
// Priority: settings.ShellPath > $SHELL > /bin/sh
func ResolveShell(s *settings.Settings) string {
	if s != nil && s.ShellPath != "" {
		return s.ShellPath
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

// EnsureEssentialPaths returns a PATH string with essential directories appended
// if they are not already present.
func EnsureEssentialPaths(path string) string {
	parts := strings.Split(path, ":")
	existing := make(map[string]bool, len(parts))
	for _, p := range parts {
		existing[p] = true
	}

	var toAdd []string
	for _, ep := range EssentialPaths {
		if !existing[ep] {
			toAdd = append(toAdd, ep)
		}
	}

	if len(toAdd) == 0 {
		return path
	}

	if path == "" {
		return strings.Join(toAdd, ":")
	}
	return path + ":" + strings.Join(toAdd, ":")
}

// ProfileSourceCmd returns shell commands to source system and user profiles
// for the given shell type. All source commands suppress errors with 2>/dev/null.
func ProfileSourceCmd(st ShellType) string {
	switch st {
	case Zsh:
		parts := []string{}
		if runtime.GOOS == "darwin" {
			// macOS: /etc/zprofile runs path_helper which sets up standard PATH
			parts = append(parts, "source /etc/zprofile 2>/dev/null")
		}
		parts = append(parts,
			"source ~/.zprofile 2>/dev/null",
			"source ~/.zshrc 2>/dev/null",
		)
		return strings.Join(parts, "; ")
	case Bash:
		return "source /etc/profile 2>/dev/null; source ~/.bash_profile 2>/dev/null; source ~/.bashrc 2>/dev/null"
	case Fish:
		// fish -c auto-loads config.fish
		return ""
	case Sh:
		// Use POSIX-compatible . instead of source for sh/dash compatibility
		return ". /etc/profile 2>/dev/null; . ~/.profile 2>/dev/null"
	default:
		return ""
	}
}

// pathSafetyNet returns a shell snippet that ensures essential paths are in PATH
// even if profile sourcing overwrote them. Shell-type-aware: returns POSIX syntax
// for bash/zsh/sh, fish syntax for fish, or empty for unknown shells (which rely
// solely on cmd.Env).
func pathSafetyNet(st ShellType) string {
	essentials := strings.Join(EssentialPaths, ":")
	switch st {
	case Zsh, Bash, Sh, Unknown:
		// ${PATH:+$PATH:} avoids a leading colon when PATH is empty
		return `export PATH="${PATH:+$PATH:}` + essentials + `"`
	case Fish:
		// Fish uses its own syntax for PATH manipulation
		return "set -gx PATH $PATH " + strings.Join(EssentialPaths, " ")
	default:
		return ""
	}
}

// BuildShellCmd assembles the full shell command string:
// profile sourcing + PATH safety net + the actual command.
func BuildShellCmd(st ShellType, command string) string {
	var parts []string

	if prof := ProfileSourceCmd(st); prof != "" {
		parts = append(parts, prof)
	}

	if net := pathSafetyNet(st); net != "" {
		parts = append(parts, net)
	}

	parts = append(parts, command)

	if st == Fish {
		// Fish uses "; and" for command chaining but plain ";" works too
		return strings.Join(parts, "; ")
	}
	return strings.Join(parts, "; ")
}

// CommandConfig holds all parameters needed to build an exec.Cmd.
type CommandConfig struct {
	Settings *settings.Settings
	Command  string // the user-facing command to run
	WorkDir  string // working directory (empty = cwd)
}

// BuildEnv returns a copy of os.Environ() with essential paths ensured in PATH
// and any custom EnvironmentVars from settings merged in. If a custom env var
// has the key "PATH", essential paths are ensured on that value too.
func BuildEnv(s *settings.Settings) []string {
	env := os.Environ()

	// Find and fix PATH
	pathFixed := false
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + EnsureEssentialPaths(e[5:])
			pathFixed = true
			break
		}
	}
	if !pathFixed {
		env = append(env, "PATH="+strings.Join(EssentialPaths, ":"))
	}

	// Merge custom environment variables from settings.
	// Replace existing keys rather than appending duplicates.
	// For PATH, ensure essential paths are preserved.
	if s != nil && len(s.EnvironmentVars) > 0 {
		for k, v := range s.EnvironmentVars {
			entry := k + "=" + v
			if k == "PATH" {
				entry = "PATH=" + EnsureEssentialPaths(v)
			}
			// Replace existing key if found
			prefix := k + "="
			replaced := false
			for i, e := range env {
				if strings.HasPrefix(e, prefix) {
					env[i] = entry
					replaced = true
					break
				}
			}
			if !replaced {
				env = append(env, entry)
			}
		}
	}

	return env
}

// CreateCommand builds a fully configured *exec.Cmd that:
//  1. Uses the resolved shell from settings / $SHELL / /bin/sh
//  2. Sources appropriate profiles for the detected shell type
//  3. Ensures essential system paths in both cmd.Env and the shell command string
//  4. Merges custom EnvironmentVars from settings
//  5. Sets the working directory
func CreateCommand(cfg CommandConfig) *exec.Cmd {
	shellPath := ResolveShell(cfg.Settings)
	st := DetectShell(shellPath)

	fullCmd := BuildShellCmd(st, cfg.Command)
	cmd := exec.Command(shellPath, "-c", fullCmd)

	cmd.Env = BuildEnv(cfg.Settings)

	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	} else if cwd, err := os.Getwd(); err == nil {
		cmd.Dir = cwd
	}

	return cmd
}
