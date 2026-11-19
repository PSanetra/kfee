package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The configuration is stored in two files: config.yaml holds the settings not related to
// connecting to clusters, connections.yaml, readable by the user only, holds how to connect to them.

// configFile is the content of config.yaml.
type configFile struct {
	Clusters              []clusterFile `yaml:"clusters"`
	PlainFonts            bool          `yaml:"plainFonts"`
	Theme                 string        `yaml:"theme,omitempty"`
	LLM                   *LLMConfig    `yaml:"llm,omitempty"`
	TransformationHistory int           `yaml:"transformationHistory,omitempty"`
}

type clusterFile struct {
	Name     string          `yaml:"name"`
	Color    string          `yaml:"color"`
	Active   bool            `yaml:"active"`
	Protobuf *ProtobufConfig `yaml:"protobuf,omitempty"`
}

// connectionsFile is the content of connections.yaml.
type connectionsFile struct {
	// Clusters maps cluster names to their connection settings
	Clusters map[string]connection `yaml:"clusters"`
}

type connection struct {
	BootstrapServers     []string              `yaml:"servers"`
	SASLConfig           SASLConfig            `yaml:"sasl"`
	TLSConfig            TLSConfig             `yaml:"tls"`
	SchemaRegistry       *SchemaRegistryConfig `yaml:"schemaRegistry,omitempty"`
	KafkaConnectClusters []KafkaConnectConfig  `yaml:"kafkaConnectClusters,omitempty"`
}

// connectionKeys are the cluster settings that belong into connections.yaml.
var connectionKeys = []string{"servers", "sasl", "tls", "schemaRegistry", "kafkaConnectClusters"}

func splitConfig(c *Config) (configFile, connectionsFile) {
	cfg := configFile{PlainFonts: c.PlainFonts, Theme: c.Theme, LLM: c.LLM, TransformationHistory: c.TransformationHistory}
	connections := connectionsFile{Clusters: map[string]connection{}}
	for _, cluster := range c.Clusters {
		cfg.Clusters = append(cfg.Clusters, clusterFile{
			Name:     cluster.Name,
			Color:    cluster.Color,
			Active:   cluster.Active,
			Protobuf: cluster.Protobuf,
		})
		connections.Clusters[cluster.Name] = connection{
			BootstrapServers:     cluster.BootstrapServers,
			SASLConfig:           cluster.SASLConfig,
			TLSConfig:            cluster.TLSConfig,
			SchemaRegistry:       cluster.SchemaRegistry,
			KafkaConnectClusters: cluster.KafkaConnectClusters,
		}
	}
	return cfg, connections
}

// joinConfig combines both files. Clusters only present in connections.yaml are added as well.
func joinConfig(cfg configFile, connections connectionsFile) *Config {
	c := &Config{PlainFonts: cfg.PlainFonts, Theme: cfg.Theme, LLM: cfg.LLM, TransformationHistory: cfg.TransformationHistory}
	seen := map[string]bool{}
	for _, cf := range cfg.Clusters {
		seen[cf.Name] = true
		c.Clusters = append(c.Clusters, withConnection(Cluster{
			Name:     cf.Name,
			Color:    cf.Color,
			Active:   cf.Active,
			Protobuf: cf.Protobuf,
		}, connections.Clusters[cf.Name]))
	}

	var connectionsOnly []string
	for name := range connections.Clusters {
		if !seen[name] {
			connectionsOnly = append(connectionsOnly, name)
		}
	}
	sort.Strings(connectionsOnly)
	for _, name := range connectionsOnly {
		c.Clusters = append(c.Clusters, withConnection(Cluster{Name: name}, connections.Clusters[name]))
	}
	return c
}

func withConnection(c Cluster, conn connection) Cluster {
	c.BootstrapServers = conn.BootstrapServers
	c.SASLConfig = conn.SASLConfig
	c.TLSConfig = conn.TLSConfig
	c.SchemaRegistry = conn.SchemaRegistry
	c.KafkaConnectClusters = conn.KafkaConnectClusters
	return c
}

// checkNoConnections fails when config.yaml contains connection settings, which are only
// read from connections.yaml.
func checkNoConnections(configYaml []byte, connectionsPath string) error {
	var c struct {
		Clusters []map[string]any `yaml:"clusters"`
	}
	if err := yaml.Unmarshal(configYaml, &c); err != nil {
		return err
	}
	var clusters []string
	for _, cluster := range c.Clusters {
		for _, key := range connectionKeys {
			if _, ok := cluster[key]; ok {
				clusters = append(clusters, fmt.Sprint(cluster["name"]))
				break
			}
		}
	}
	if len(clusters) == 0 {
		return nil
	}
	return fmt.Errorf(`config.yaml contains connection settings of cluster %s.
They are only read from %s now. Move %s of every cluster there, e.g.:

clusters:
    %s:
        servers:
            - ...
        sasl:
            ...`,
		strings.Join(clusters, ", "), connectionsPath, strings.Join(connectionKeys, ", "), clusters[0])
}

func readConnections(path string) (connectionsFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return connectionsFile{}, nil
	}
	if err != nil {
		return connectionsFile{}, err
	}
	var connections connectionsFile
	if err := yaml.Unmarshal(data, &connections); err != nil {
		return connectionsFile{}, err
	}
	return connections, nil
}

// writeConnections atomically replaces the connections file, readable by the user only.
func writeConnections(path string, connections connectionsFile) error {
	if len(connections.Clusters) == 0 {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return nil
		}
	}
	data, err := yaml.Marshal(connections)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// temp files are created readable by the user only, independent of the umask
	f, err := os.CreateTemp(filepath.Dir(path), ".connections-*.yaml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
