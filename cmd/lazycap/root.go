package lazycap

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"lazycap/internal/cap"
	"lazycap/internal/plugin"
	"lazycap/internal/plugins"
	"lazycap/internal/ui"
)

var (
	appVersion string
	appCommit  string
	appDate    string
	demoMode   bool
)

var rootCmd = &cobra.Command{
	Use:   "lazycap",
	Short: "A slick terminal UI for Capacitor & Ionic development",
	Long: `lazycap is a terminal UI for Capacitor/Ionic mobile development.
Manage devices, emulators, builds, and live reload from one beautiful interface.

Navigate to your Capacitor project directory and run 'lazycap' to get started.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if demoMode {
			return runDemoMode()
		}
		return runApp()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("lazycap %s\n", appVersion)
		fmt.Printf("  commit: %s\n", appCommit)
		fmt.Printf("  built:  %s\n", appDate)
	},
}

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List available devices and emulators",
	RunE: func(cmd *cobra.Command, args []string) error {
		devices, err := cap.ListDevices()
		if err != nil {
			return err
		}
		for _, d := range devices {
			status := "offline"
			if d.Online {
				status = "online"
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", d.ID, d.Name, d.Platform, status)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(devicesCmd)

	// Global flags
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file (default: .lazycap.yaml)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.Flags().BoolVar(&demoMode, "demo", false, "run in demo mode with mock data (for screenshots)")
}

func Execute(version, commit, date string) error {
	appVersion = version
	appCommit = commit
	appDate = date
	return rootCmd.Execute()
}

func runDemoMode() error {
	// Create mock project
	project := &cap.Project{
		Name:       "my-awesome-app",
		RootDir:    "/demo",
		HasIOS:     true,
		HasAndroid: true,
	}

	// Register plugins (still useful for demo)
	plugins.RegisterAll()
	pluginManager := plugin.NewManager()
	appContext := plugin.NewAppContext(pluginManager)
	appContext.SetProject(project)

	// Create model with demo devices
	model := ui.NewDemoModel(project, pluginManager, appContext)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running demo: %w", err)
	}

	return nil
}

func runApp() error {
	// Check if we're in a Capacitor project
	if !cap.IsCapacitorProject() {
		return fmt.Errorf("not a Capacitor project (no capacitor.config.ts/js/json found)")
	}

	// Load project info
	project, err := cap.LoadProject()
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	// Register all built-in plugins
	if err := plugins.RegisterAll(); err != nil {
		return fmt.Errorf("failed to register plugins: %w", err)
	}

	// Create plugin manager
	pluginManager := plugin.NewManager()

	// Create plugin context (bridges plugins with UI)
	appContext := plugin.NewAppContext(pluginManager)
	appContext.SetProject(project)

	// Initialize and run the TUI with plugin support
	model := ui.NewModelWithPlugins(project, pluginManager, appContext)
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Handle graceful shutdown for plugins
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		pluginManager.StopAll()
		os.Exit(0)
	}()

	// Initialize all plugins with context
	if err := pluginManager.InitAll(appContext); err != nil {
		// Log but don't fail - plugins are optional
		fmt.Fprintf(os.Stderr, "Warning: some plugins failed to initialize: %v\n", err)
	}

	// Start auto-start plugins
	pluginManager.StartAutoStart()

	// Run the TUI
	if _, err := p.Run(); err != nil {
		pluginManager.StopAll()
		return fmt.Errorf("error running app: %w", err)
	}

	// Stop all plugins on exit
	pluginManager.StopAll()

	return nil
}
