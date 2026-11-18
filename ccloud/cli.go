// Package ccloud configures Confluent Cloud clusters using the Confluent CLI.
package ccloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner runs the Confluent CLI with the given arguments and returns its stdout.
type Runner func(args ...string) ([]byte, error)

// ExecRunner runs the given Confluent CLI binary.
func ExecRunner(binary string) Runner {
	return func(args ...string) ([]byte, error) {
		cmd := exec.Command(binary, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return nil, fmt.Errorf("%s not found, install the Confluent CLI first", binary)
			}
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return nil, fmt.Errorf("%s %s: %s", binary, strings.Join(args, " "), msg)
			}
			return nil, fmt.Errorf("%s %s: %w", binary, strings.Join(args, " "), err)
		}
		return stdout.Bytes(), nil
	}
}

// Login runs `confluent login` interactively.
func Login(binary string) error {
	cmd := exec.Command(binary, "login")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

type Environment struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsCurrent bool   `json:"is_current"`
}

type KafkaCluster struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Cloud     string `json:"cloud"`
	Region    string `json:"region"`
	Status    string `json:"status"`
	Endpoint  string `json:"endpoint"`
	IsCurrent bool   `json:"is_current"`
}

// BootstrapServer returns the cluster endpoint without the protocol, e.g. pkc-123.eu-central-1.aws.confluent.cloud:9092.
func (c KafkaCluster) BootstrapServer() string {
	if _, server, ok := strings.Cut(c.Endpoint, "://"); ok {
		return server
	}
	return c.Endpoint
}

type SchemaRegistry struct {
	ID          string `json:"cluster"`
	EndpointURL string `json:"endpoint_url"`
}

type APIKey struct {
	Key    string `json:"api_key"`
	Secret string `json:"api_secret"`
}

// KeyDescription is the description of API keys created by ktea.
func KeyDescription(now time.Time) string {
	return "ktea " + now.Format("2006-01-02 15:04:05")
}

// CLI calls the Confluent CLI.
type CLI struct {
	run Runner
}

func NewCLI(run Runner) CLI {
	return CLI{run: run}
}

func (c CLI) Environments() ([]Environment, error) {
	var envs []Environment
	return envs, c.runJSON(&envs, "environment", "list")
}

func (c CLI) KafkaClusters(environment string) ([]KafkaCluster, error) {
	var clusters []KafkaCluster
	return clusters, c.runJSON(&clusters, "kafka", "cluster", "list", "--environment", environment)
}

// SchemaRegistry returns the Schema Registry of the environment.
func (c CLI) SchemaRegistry(environment string) (SchemaRegistry, error) {
	var sr SchemaRegistry
	if err := c.runJSON(&sr, "schema-registry", "cluster", "describe", "--environment", environment); err != nil {
		return SchemaRegistry{}, err
	}
	if sr.ID == "" || sr.EndpointURL == "" {
		return SchemaRegistry{}, errors.New("the environment has no Schema Registry")
	}
	return sr, nil
}

// CreateAPIKey creates an API key for the given resource, e.g. a Kafka or Schema Registry cluster.
func (c CLI) CreateAPIKey(environment, resource, description string) (APIKey, error) {
	var key APIKey
	if err := c.runJSON(&key,
		"api-key", "create",
		"--resource", resource,
		"--environment", environment,
		"--description", description,
	); err != nil {
		return APIKey{}, err
	}
	if key.Key == "" || key.Secret == "" {
		return APIKey{}, fmt.Errorf("the Confluent CLI returned no API key for %s", resource)
	}
	return key, nil
}

func (c CLI) runJSON(out any, args ...string) error {
	data, err := c.run(append(args, "--output", "json")...)
	if err != nil {
		return err
	}
	// skip notices printed before the json, e.g. about available updates
	if i := bytes.IndexAny(data, "[{"); i > 0 {
		data = data[i:]
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("unexpected output of confluent %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
