package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"sync"
)

// Plugin represents a CLI plugin
type Plugin struct {
	Name        string
	Version     string
	Description string
	Commands    map[string]*Command
	Hooks       map[string][]HookFunc
}

// HookFunc represents a hook function
type HookFunc func(args ...interface{}) error

// PluginManager manages CLI plugins
type PluginManager struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
	hooks   map[string][]HookFunc
}

// NewPluginManager creates a new plugin manager
func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins: make(map[string]*Plugin),
		hooks:   make(map[string][]HookFunc),
	}
}

// RegisterPlugin registers a plugin
func (pm *PluginManager) RegisterPlugin(p *Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.plugins[p.Name]; exists {
		return fmt.Errorf("plugin %s already registered", p.Name)
	}

	pm.plugins[p.Name] = p

	// Register plugin commands
	for name, cmd := range p.Commands {
		RootCommand.Subcommands[name] = cmd
	}

	// Register plugin hooks
	for hookName, hooks := range p.Hooks {
		pm.hooks[hookName] = append(pm.hooks[hookName], hooks...)
	}

	return nil
}

// UnregisterPlugin unregisters a plugin
func (pm *PluginManager) UnregisterPlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, exists := pm.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// Remove plugin commands
	for cmdName := range p.Commands {
		delete(RootCommand.Subcommands, cmdName)
	}

	delete(pm.plugins, name)
	return nil
}

// GetPlugin retrieves a plugin by name
func (pm *PluginManager) GetPlugin(name string) (*Plugin, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	p, exists := pm.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}
	return p, nil
}

// ListPlugins lists all registered plugins
func (pm *PluginManager) ListPlugins() []*Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var plugins []*Plugin
	for _, p := range pm.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// ExecuteHook executes all registered hooks for a given event
func (pm *PluginManager) ExecuteHook(hookName string, args ...interface{}) error {
	pm.mu.RLock()
	hooks := pm.hooks[hookName]
	pm.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook(args...); err != nil {
			return err
		}
	}
	return nil
}

// LoadPlugin loads a plugin from a shared library
func (pm *PluginManager) LoadPlugin(path string) error {
	p, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("failed to load plugin %s: %w", path, err)
	}

	// Look for plugin symbol
	sym, err := p.Lookup("Plugin")
	if err != nil {
		return fmt.Errorf("plugin %s does not export Plugin symbol", path)
	}

	pluginInstance, ok := sym.(*Plugin)
	if !ok {
		return fmt.Errorf("plugin %s has invalid Plugin symbol", path)
	}

	return pm.RegisterPlugin(pluginInstance)
}

// LoadPluginsFromDir loads all plugins from a directory
func (pm *PluginManager) LoadPluginsFromDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".so" && ext != ".dylib" && ext != ".dll" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if err := pm.LoadPlugin(path); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load plugin %s: %v\n", path, err)
		}
	}

	return nil
}

// Global plugin manager instance
var globalPluginManager = NewPluginManager()

// GetPluginManager returns the global plugin manager
func GetPluginManager() *PluginManager {
	return globalPluginManager
}

// Built-in hooks
const (
	HookPreCommand  = "pre_command"
	HookPostCommand = "post_command"
	HookOnError     = "on_error"
	HookOnConfig    = "on_config"
)

// PluginCommand handles plugin management
var PluginCommand *Command

func init() {
	PluginCommand = &Command{
		Name:    "plugin",
		Aliases: []string{"plug"},
		Short:   "Manage plugins",
		Long:    "Manage CLI plugins including listing, loading, and unloading plugins.",
		Usage:   "gotunnel plugin <list|load|unload> [options]",
		Subcommands: map[string]*Command{
			"list": {
				Name:  "list",
				Short: "List installed plugins",
				Run:   runPluginList,
			},
			"load": {
				Name:  "load",
				Short: "Load a plugin from file",
				Run:   runPluginLoad,
			},
			"unload": {
				Name:  "unload",
				Short: "Unload a plugin",
				Run:   runPluginUnload,
			},
		},
		Run: runPluginCmd,
	}
}

func runPluginCmd(args []string) error {
	if len(args) == 0 {
		printCommandHelp(PluginCommand)
		return nil
	}
	return fmt.Errorf("unknown plugin subcommand: %s", args[0])
}

func runPluginList(args []string) error {
	pm := GetPluginManager()
	plugins := pm.ListPlugins()

	if len(plugins) == 0 {
		fmt.Println("No plugins installed")
		return nil
	}

	fmt.Println("Installed plugins:")
	for _, p := range plugins {
		fmt.Printf("  %s (v%s): %s\n", p.Name, p.Version, p.Description)
		if len(p.Commands) > 0 {
			var cmds []string
			for cmd := range p.Commands {
				cmds = append(cmds, cmd)
			}
			fmt.Printf("    Commands: %s\n", strings.Join(cmds, ", "))
		}
	}
	return nil
}

func runPluginLoad(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gotunnel plugin load <path>")
	}

	path := args[0]
	pm := GetPluginManager()

	if err := pm.LoadPlugin(path); err != nil {
		return err
	}

	fmt.Printf("Plugin loaded from %s\n", path)
	return nil
}

func runPluginUnload(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gotunnel plugin unload <name>")
	}

	name := args[0]
	pm := GetPluginManager()

	if err := pm.UnregisterPlugin(name); err != nil {
		return err
	}

	fmt.Printf("Plugin %s unloaded\n", name)
	return nil
}

// Plugin registration is handled in root.go init()
