package config

import (
	"charm.land/log/v2"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

type IO interface {
	write(config *Config) error
	read() (*Config, error)
}

type defaultConfigIO struct {
	configPath      string
	connectionsPath string
}

func NewDefaultIO() IO {
	return newFileIO(configPath(), ConnectionsPath())
}

func newFileIO(configPath, connectionsPath string) IO {
	return &defaultConfigIO{configPath: configPath, connectionsPath: connectionsPath}
}

func (c *defaultConfigIO) read() (*Config, error) {
	// Check if the file exists
	if _, err := os.Stat(c.configPath); os.IsNotExist(err) {
		// Ensure the directory structure exists, only accessible by the user as it holds connections.yaml
		if err := os.MkdirAll(filepath.Dir(c.configPath), 0700); err != nil {
			return nil, err
		}

		// Create the file
		if _, err := os.Create(c.configPath); err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return nil, err
	}
	if err := checkNoConnections(data, c.connectionsPath); err != nil {
		return nil, err
	}
	var cfg configFile
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	connections, err := readConnections(c.connectionsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read %s: %w", c.connectionsPath, err)
	}
	config := joinConfig(cfg, connections)
	// rules stored before they had IDs get one now, the history of a rule depends on it
	if config.ensureRuleIDs() {
		if err := c.write(config); err != nil {
			return nil, err
		}
	}
	return config, nil
}

func (c *defaultConfigIO) write(config *Config) error {
	cfg, connections := splitConfig(config)

	if err := os.MkdirAll(filepath.Dir(c.configPath), 0700); err != nil {
		log.Fatalf("Error creating config directory: %v", err)
		return err
	}
	if err := writeConnections(c.connectionsPath, connections); err != nil {
		log.Fatalf("Error writing connections file: %v", err)
		return err
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatalf("Error marshalling config: %v", err)
		return err
	}

	err = os.WriteFile(c.configPath, out, 0644)
	if err != nil {
		log.Fatalf("Error writing config file: %v", err)
		return err
	}

	return nil
}

func configPath() string {
	return filepath.Join(configDir(), "config.yaml")
}

// Path is the path of the configuration file.
func Path() string {
	return configPath()
}

// ConnectionsPath is the path of the file holding the connection settings of the clusters, readable by the user only.
func ConnectionsPath() string {
	return filepath.Join(configDir(), "connections.yaml")
}

// ThemesDir is where user themes live, one <name>.yaml per theme.
func ThemesDir() string {
	return filepath.Join(configDir(), "themes")
}

// TransformationsDir is where the applied transformations are kept, one file per cluster and topic.
// It is state rather than configuration, hence in $XDG_DATA_HOME.
func TransformationsDir() string {
	return resolveDataDir(os.Getenv("XDG_DATA_HOME"), os.UserHomeDir)
}

func resolveDataDir(xdgDataHome string, homeDir func() (string, error)) string {
	// relative paths are invalid according to the XDG Base Directory Specification
	if filepath.IsAbs(xdgDataHome) {
		return filepath.Join(xdgDataHome, "kfee", "transformations")
	}
	home, err := homeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(-1)
	}
	return filepath.Join(home, ".local", "share", "kfee", "transformations")
}

// configDir is $XDG_CONFIG_HOME/kfee when XDG_CONFIG_HOME is set to an absolute path,
// ~/.config/kfee otherwise, also on macOS as common for command line tools.
func configDir() string {
	return resolveConfigDir(os.Getenv("XDG_CONFIG_HOME"), os.UserHomeDir)
}

func resolveConfigDir(xdgConfigHome string, homeDir func() (string, error)) string {
	// relative paths are invalid according to the XDG Base Directory Specification
	if filepath.IsAbs(xdgConfigHome) {
		return filepath.Join(xdgConfigHome, "kfee")
	}
	home, err := homeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(-1)
	}
	return filepath.Join(home, ".config", "kfee")
}
