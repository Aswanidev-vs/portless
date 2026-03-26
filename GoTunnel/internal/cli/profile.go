package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile represents an environment configuration profile
type Profile struct {
	Name        string                 `yaml:"name" json:"name"`
	Description string                 `yaml:"description" json:"description"`
	Environment string                 `yaml:"environment" json:"environment"`
	Config      map[string]interface{} `yaml:"config" json:"config"`
	Variables   map[string]string      `yaml:"variables" json:"variables"`
}

// ProfileManager manages environment profiles
type ProfileManager struct {
	profilesDir string
	profiles    map[string]*Profile
}

// NewProfileManager creates a new profile manager
func NewProfileManager() *ProfileManager {
	homeDir, _ := os.UserHomeDir()
	profilesDir := filepath.Join(homeDir, ".gotunnel", "profiles")
	return &ProfileManager{
		profilesDir: profilesDir,
		profiles:    make(map[string]*Profile),
	}
}

// LoadProfiles loads all profiles from the profiles directory
func (pm *ProfileManager) LoadProfiles() error {
	if _, err := os.Stat(pm.profilesDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(pm.profilesDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ext)
		profile, err := pm.LoadProfile(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load profile %s: %v\n", name, err)
			continue
		}

		pm.profiles[name] = profile
	}

	return nil
}

// LoadProfile loads a specific profile
func (pm *ProfileManager) LoadProfile(name string) (*Profile, error) {
	path := filepath.Join(pm.profilesDir, name+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(pm.profilesDir, name+".yml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, err
	}

	profile.Name = name
	return &profile, nil
}

// SaveProfile saves a profile
func (pm *ProfileManager) SaveProfile(profile *Profile) error {
	if err := os.MkdirAll(pm.profilesDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}

	path := filepath.Join(pm.profilesDir, profile.Name+".yaml")
	return os.WriteFile(path, data, 0644)
}

// GetProfile retrieves a profile by name
func (pm *ProfileManager) GetProfile(name string) (*Profile, error) {
	profile, exists := pm.profiles[name]
	if !exists {
		return nil, fmt.Errorf("profile %s not found", name)
	}
	return profile, nil
}

// ListProfiles lists all available profiles
func (pm *ProfileManager) ListProfiles() []*Profile {
	var profiles []*Profile
	for _, p := range pm.profiles {
		profiles = append(profiles, p)
	}
	return profiles
}

// DeleteProfile deletes a profile
func (pm *ProfileManager) DeleteProfile(name string) error {
	path := filepath.Join(pm.profilesDir, name+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(pm.profilesDir, name+".yml")
	}

	if err := os.Remove(path); err != nil {
		return err
	}

	delete(pm.profiles, name)
	return nil
}

// ApplyProfile applies a profile's configuration
func (pm *ProfileManager) ApplyProfile(name string) error {
	profile, err := pm.GetProfile(name)
	if err != nil {
		return err
	}

	// Set environment variables from profile
	for key, value := range profile.Variables {
		os.Setenv(key, value)
	}

	return nil
}

// Global profile manager instance
var globalProfileManager = NewProfileManager()

// GetProfileManager returns the global profile manager
func GetProfileManager() *ProfileManager {
	return globalProfileManager
}

// ProfileCommand handles profile management
var ProfileCommand *Command

func init() {
	ProfileCommand = &Command{
		Name:    "profile",
		Aliases: []string{"env"},
		Short:   "Manage environment profiles",
		Long:    "Manage environment configuration profiles for different deployment scenarios (dev, staging, prod).",
		Usage:   "gotunnel profile <list|show|create|delete|apply> [options]",
		Subcommands: map[string]*Command{
			"list": {
				Name:  "list",
				Short: "List available profiles",
				Run:   runProfileList,
			},
			"show": {
				Name:  "show",
				Short: "Show profile details",
				Run:   runProfileShow,
			},
			"create": {
				Name:  "create",
				Short: "Create a new profile",
				Run:   runProfileCreate,
			},
			"delete": {
				Name:  "delete",
				Short: "Delete a profile",
				Run:   runProfileDelete,
			},
			"apply": {
				Name:  "apply",
				Short: "Apply a profile",
				Run:   runProfileApply,
			},
		},
		Run: runProfileCmd,
	}
}

func runProfileCmd(args []string) error {
	if len(args) == 0 {
		printCommandHelp(ProfileCommand)
		return nil
	}
	return fmt.Errorf("unknown profile subcommand: %s", args[0])
}

func runProfileList(args []string) error {
	pm := GetProfileManager()
	_ = pm.LoadProfiles()
	profiles := pm.ListProfiles()

	if len(profiles) == 0 {
		fmt.Println("No profiles configured")
		fmt.Println("\nCreate a profile with: gotunnel profile create --name <name> --env <dev|staging|prod>")
		return nil
	}

	fmt.Println("Available profiles:")
	for _, p := range profiles {
		fmt.Printf("  %-15s %s (%s)\n", p.Name, p.Description, p.Environment)
	}
	return nil
}

func runProfileShow(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gotunnel profile show <name>")
	}

	pm := GetProfileManager()
	_ = pm.LoadProfiles()

	profile, err := pm.GetProfile(args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Profile: %s\n", profile.Name)
	fmt.Printf("Description: %s\n", profile.Description)
	fmt.Printf("Environment: %s\n", profile.Environment)

	if len(profile.Variables) > 0 {
		fmt.Println("\nEnvironment Variables:")
		for k, v := range profile.Variables {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}
	return nil
}

func runProfileCreate(args []string) error {
	name := ""
	env := "dev"
	description := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name", "-n":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "--env", "-e":
			if i+1 < len(args) {
				env = args[i+1]
				i++
			}
		case "--desc", "-d":
			if i+1 < len(args) {
				description = args[i+1]
				i++
			}
		}
	}

	if name == "" {
		return fmt.Errorf("usage: gotunnel profile create --name <name> --env <dev|staging|prod>")
	}

	profile := &Profile{
		Name:        name,
		Description: description,
		Environment: env,
		Variables:   make(map[string]string),
		Config:      make(map[string]interface{}),
	}

	pm := GetProfileManager()
	if err := pm.SaveProfile(profile); err != nil {
		return err
	}

	fmt.Printf("Profile %s created\n", name)
	return nil
}

func runProfileDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gotunnel profile delete <name>")
	}

	pm := GetProfileManager()
	if err := pm.DeleteProfile(args[0]); err != nil {
		return err
	}

	fmt.Printf("Profile %s deleted\n", args[0])
	return nil
}

func runProfileApply(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gotunnel profile apply <name>")
	}

	pm := GetProfileManager()
	_ = pm.LoadProfiles()

	if err := pm.ApplyProfile(args[0]); err != nil {
		return err
	}

	fmt.Printf("Profile %s applied\n", args[0])
	return nil
}

// Profile registration is handled in root.go init()
