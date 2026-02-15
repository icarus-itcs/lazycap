package shell

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/icarus-itcs/lazycap/internal/settings"
)

func TestDetectShell(t *testing.T) {
	tests := []struct {
		path string
		want ShellType
	}{
		{"/bin/zsh", Zsh},
		{"/usr/local/bin/zsh", Zsh},
		{"/usr/local/bin/zsh-5.9", Zsh},
		{"/bin/bash", Bash},
		{"/usr/bin/bash", Bash},
		{"/opt/homebrew/bin/bash-5.2", Bash},
		{"/usr/local/bin/fish", Fish},
		{"/usr/local/bin/fish-3.6", Fish},
		{"/bin/sh", Sh},
		{"/bin/dash", Sh},
		{"/usr/bin/dash", Sh},
		{"", Unknown},
	}

	for _, tt := range tests {
		got := DetectShell(tt.path)
		if got != tt.want {
			t.Errorf("DetectShell(%q) = %d, want %d", tt.path, got, tt.want)
		}
	}
}

func TestResolveShell(t *testing.T) {
	// With ShellPath set in settings
	s := settings.DefaultSettings()
	s.ShellPath = "/usr/local/bin/fish"
	if got := ResolveShell(s); got != "/usr/local/bin/fish" {
		t.Errorf("ResolveShell with ShellPath = %q, want /usr/local/bin/fish", got)
	}

	// With empty settings, should fall back to $SHELL
	s2 := settings.DefaultSettings()
	t.Setenv("SHELL", "/bin/bash")

	if got := ResolveShell(s2); got != "/bin/bash" {
		t.Errorf("ResolveShell with $SHELL = %q, want /bin/bash", got)
	}

	// With nil settings, should fall back to $SHELL
	if got := ResolveShell(nil); got != "/bin/bash" {
		t.Errorf("ResolveShell(nil) = %q, want /bin/bash", got)
	}

	// With empty $SHELL, should default to /bin/sh
	t.Setenv("SHELL", "")
	if got := ResolveShell(s2); got != "/bin/sh" {
		t.Errorf("ResolveShell with no SHELL = %q, want /bin/sh", got)
	}
}

func TestEnsureEssentialPaths(t *testing.T) {
	// Empty PATH gets all essential paths
	got := EnsureEssentialPaths("")
	for _, ep := range EssentialPaths {
		if !strings.Contains(got, ep) {
			t.Errorf("EnsureEssentialPaths(\"\") missing %q: %q", ep, got)
		}
	}

	// PATH already containing all essential paths is unchanged
	full := strings.Join(EssentialPaths, ":")
	got = EnsureEssentialPaths(full)
	if got != full {
		t.Errorf("EnsureEssentialPaths already complete: got %q, want %q", got, full)
	}

	// Partial PATH gets missing dirs appended
	partial := "/usr/bin:/bin"
	got = EnsureEssentialPaths(partial)
	if !strings.HasPrefix(got, partial+":") {
		t.Errorf("EnsureEssentialPaths should preserve existing path: %q", got)
	}
	if !strings.Contains(got, "/usr/local/bin") {
		t.Errorf("EnsureEssentialPaths missing /usr/local/bin: %q", got)
	}
	if !strings.Contains(got, "/usr/sbin") {
		t.Errorf("EnsureEssentialPaths missing /usr/sbin: %q", got)
	}
	if !strings.Contains(got, "/sbin") {
		t.Errorf("EnsureEssentialPaths missing /sbin: %q", got)
	}

	// Custom paths are preserved
	custom := "/opt/homebrew/bin:/usr/bin:/bin"
	got = EnsureEssentialPaths(custom)
	if !strings.HasPrefix(got, custom) {
		t.Errorf("EnsureEssentialPaths should preserve custom paths: %q", got)
	}
}

func TestProfileSourceCmd(t *testing.T) {
	// Zsh should source zprofile and zshrc
	zshProf := ProfileSourceCmd(Zsh)
	if !strings.Contains(zshProf, "~/.zprofile") {
		t.Errorf("Zsh profile missing ~/.zprofile: %q", zshProf)
	}
	if !strings.Contains(zshProf, "~/.zshrc") {
		t.Errorf("Zsh profile missing ~/.zshrc: %q", zshProf)
	}
	if runtime.GOOS == "darwin" {
		if !strings.Contains(zshProf, "/etc/zprofile") {
			t.Errorf("Zsh profile on macOS missing /etc/zprofile: %q", zshProf)
		}
	}

	// Bash should source profile files
	bashProf := ProfileSourceCmd(Bash)
	if !strings.Contains(bashProf, "/etc/profile") {
		t.Errorf("Bash profile missing /etc/profile: %q", bashProf)
	}
	if !strings.Contains(bashProf, "~/.bash_profile") {
		t.Errorf("Bash profile missing ~/.bash_profile: %q", bashProf)
	}
	if !strings.Contains(bashProf, "~/.bashrc") {
		t.Errorf("Bash profile missing ~/.bashrc: %q", bashProf)
	}

	// Fish should return empty (auto-loads config)
	if got := ProfileSourceCmd(Fish); got != "" {
		t.Errorf("Fish profile should be empty, got %q", got)
	}

	// Sh should use POSIX . instead of source
	shProf := ProfileSourceCmd(Sh)
	if !strings.Contains(shProf, ". /etc/profile") {
		t.Errorf("Sh profile missing '. /etc/profile': %q", shProf)
	}
	if !strings.Contains(shProf, ". ~/.profile") {
		t.Errorf("Sh profile missing '. ~/.profile': %q", shProf)
	}
	if strings.Contains(shProf, "source") {
		t.Errorf("Sh profile should use . not source: %q", shProf)
	}

	// Unknown should return empty
	if got := ProfileSourceCmd(Unknown); got != "" {
		t.Errorf("Unknown profile should be empty, got %q", got)
	}
}

func TestBuildShellCmd(t *testing.T) {
	cmd := BuildShellCmd(Zsh, "echo hello")

	// Should contain the profile sourcing
	if !strings.Contains(cmd, "~/.zshrc") {
		t.Errorf("BuildShellCmd(Zsh) missing profile sourcing: %q", cmd)
	}

	// Should contain the POSIX PATH safety net
	if !strings.Contains(cmd, "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin") {
		t.Errorf("BuildShellCmd(Zsh) missing PATH safety net: %q", cmd)
	}
	if !strings.Contains(cmd, "export PATH=") {
		t.Errorf("BuildShellCmd(Zsh) should use export PATH: %q", cmd)
	}

	// Should end with the actual command
	if !strings.HasSuffix(cmd, "; echo hello") {
		t.Errorf("BuildShellCmd(Zsh) should end with command: %q", cmd)
	}

	// Fish should use fish-compatible PATH syntax
	fishCmd := BuildShellCmd(Fish, "echo hello")
	if strings.Contains(fishCmd, "export PATH=") {
		t.Errorf("BuildShellCmd(Fish) should NOT use export PATH: %q", fishCmd)
	}
	if !strings.Contains(fishCmd, "set -gx PATH") {
		t.Errorf("BuildShellCmd(Fish) should use 'set -gx PATH': %q", fishCmd)
	}
	if !strings.HasSuffix(fishCmd, "; echo hello") {
		t.Errorf("BuildShellCmd(Fish) should end with command: %q", fishCmd)
	}
}

func TestBuildEnv(t *testing.T) {
	s := settings.DefaultSettings()
	s.EnvironmentVars = map[string]string{
		"MY_VAR": "hello",
	}

	env := BuildEnv(s)

	// Should contain PATH with essential dirs
	foundPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			foundPath = true
			val := e[5:]
			for _, ep := range EssentialPaths {
				if !strings.Contains(val, ep) {
					t.Errorf("BuildEnv PATH missing %q: %q", ep, val)
				}
			}
		}
	}
	if !foundPath {
		t.Error("BuildEnv missing PATH entry")
	}

	// Should contain custom env var
	found := false
	for _, e := range env {
		if e == "MY_VAR=hello" {
			found = true
			break
		}
	}
	if !found {
		t.Error("BuildEnv missing custom env var MY_VAR=hello")
	}

	// With nil settings should still work
	env2 := BuildEnv(nil)
	if len(env2) == 0 {
		t.Error("BuildEnv(nil) returned empty env")
	}
}

func TestBuildEnvCustomPathPreservesEssentials(t *testing.T) {
	s := settings.DefaultSettings()
	s.EnvironmentVars = map[string]string{
		"PATH": "/opt/custom/bin",
	}

	env := BuildEnv(s)

	// Find PATH entry - should have essential paths merged
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			val := e[5:]
			if !strings.Contains(val, "/opt/custom/bin") {
				t.Errorf("BuildEnv should preserve custom PATH value: %q", val)
			}
			for _, ep := range EssentialPaths {
				if !strings.Contains(val, ep) {
					t.Errorf("BuildEnv custom PATH should still contain %q: %q", ep, val)
				}
			}
			return
		}
	}
	t.Error("BuildEnv missing PATH entry")
}

func TestBuildEnvReplacesExistingKeys(t *testing.T) {
	// Set a known env var so we can verify replacement vs duplication
	t.Setenv("_SHELL_TEST_VAR", "original")

	s := settings.DefaultSettings()
	s.EnvironmentVars = map[string]string{
		"_SHELL_TEST_VAR": "replaced",
	}

	env := BuildEnv(s)

	count := 0
	for _, e := range env {
		if strings.HasPrefix(e, "_SHELL_TEST_VAR=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("BuildEnv should replace existing key, got %d entries for _SHELL_TEST_VAR", count)
	}

	// Verify it's the replaced value
	for _, e := range env {
		if e == "_SHELL_TEST_VAR=replaced" {
			return
		}
	}
	t.Error("BuildEnv did not replace _SHELL_TEST_VAR with new value")
}

func TestPathSafetyNetShellAware(t *testing.T) {
	// POSIX shells should use export syntax
	posix := pathSafetyNet(Zsh)
	if !strings.Contains(posix, "export PATH=") {
		t.Errorf("pathSafetyNet(Zsh) should use export: %q", posix)
	}

	bash := pathSafetyNet(Bash)
	if !strings.Contains(bash, "export PATH=") {
		t.Errorf("pathSafetyNet(Bash) should use export: %q", bash)
	}

	// Fish should use fish syntax
	fish := pathSafetyNet(Fish)
	if strings.Contains(fish, "export") {
		t.Errorf("pathSafetyNet(Fish) should NOT use export: %q", fish)
	}
	if !strings.Contains(fish, "set -gx PATH") {
		t.Errorf("pathSafetyNet(Fish) should use 'set -gx PATH': %q", fish)
	}
}

// Verify that SHELL env var is properly restored after TestResolveShell
// by using t.Setenv which auto-restores.
func TestResolveShellDoesNotLeakEnv(t *testing.T) {
	original := os.Getenv("SHELL")
	t.Setenv("SHELL", "/test/shell")
	_ = ResolveShell(nil)
	// t.Setenv cleanup happens automatically
	// This test just validates the test infrastructure works
	if os.Getenv("SHELL") != "/test/shell" {
		t.Error("t.Setenv did not set SHELL correctly during test")
	}
	_ = original // Will be restored by t.Cleanup
}
