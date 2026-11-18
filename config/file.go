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
	return joinConfig(cfg, connections), nil
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

func configDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(-1)
	}
	return filepath.Join(homeDir, ".config", "ktea")
}
