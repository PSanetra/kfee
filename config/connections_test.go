package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func testIO(t *testing.T) (IO, string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "kfee")
	configFile := filepath.Join(dir, "config.yaml")
	connectionsFile := filepath.Join(dir, "connections.yaml")
	return newFileIO(configFile, connectionsFile), configFile, connectionsFile
}

func prdCluster() Cluster {
	return Cluster{
		Name:             "prd",
		Color:            "#00FF00",
		Active:           true,
		BootstrapServers: []string{"pkc-1:9092"},
		SASLConfig:       SASLConfig{Username: "KEY", Password: "kafka-secret", AuthMethod: AuthMethodSASLPlaintext},
		TLSConfig:        TLSConfig{Enable: true},
		SchemaRegistry:   &SchemaRegistryConfig{Url: "https://sr", Username: "SR-KEY", Password: "sr-secret"},
		KafkaConnectClusters: []KafkaConnectConfig{
			{Name: "connect", Url: "https://connect", Username: ptr("admin"), Password: ptr("connect-secret")},
		},
		Protobuf: &ProtobufConfig{Topics: []ProtobufTopicConfig{{
			Topic: "orders", ProtoDir: "~/protos", Rules: []ProtobufRule{{MessageType: "acme.Order"}},
		}}},
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func writeFile(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), perm))
}

func assertPerm(t *testing.T, path string, perm os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, perm, info.Mode().Perm(), path)
}

func TestConfigFiles(t *testing.T) {
	t.Run("stores connection settings in connections.yaml only", func(t *testing.T) {
		io, configFile, connectionsFile := testIO(t)

		require.NoError(t, io.write(&Config{Theme: "nord", Clusters: []Cluster{prdCluster()}}))

		assert.Equal(t, `clusters:
    - name: prd
      color: '#00FF00'
      active: true
      protobuf:
        topics:
            - topic: orders
              protoDir: ~/protos
              rules:
                - messageType: acme.Order
plainFonts: false
theme: nord
`, readFile(t, configFile))
		connections := readFile(t, connectionsFile)
		for _, expected := range []string{"prd:", "pkc-1:9092", "kafka-secret", "https://sr", "sr-secret", "connect-secret"} {
			assert.Contains(t, connections, expected)
		}
		assertPerm(t, connectionsFile, 0o600)
		assertPerm(t, filepath.Dir(connectionsFile), 0o700)
	})

	t.Run("leaves no temporary files behind", func(t *testing.T) {
		io, _, connectionsFile := testIO(t)

		require.NoError(t, io.write(&Config{Clusters: []Cluster{prdCluster()}}))
		require.NoError(t, io.write(&Config{Clusters: []Cluster{prdCluster()}}))

		entries, err := os.ReadDir(filepath.Dir(connectionsFile))
		require.NoError(t, err)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		assert.ElementsMatch(t, []string{"config.yaml", "connections.yaml"}, names)
	})

	t.Run("reads both files", func(t *testing.T) {
		io, _, _ := testIO(t)
		dev := Cluster{Name: "dev", BootstrapServers: []string{"localhost:9092"}, SASLConfig: SASLConfig{AuthMethod: AuthMethodNone}}
		require.NoError(t, io.write(&Config{Theme: "nord", Clusters: []Cluster{prdCluster(), dev}}))

		cfg, err := io.read()

		require.NoError(t, err)
		assert.Equal(t, "nord", cfg.Theme)
		assert.Equal(t, []Cluster{prdCluster(), dev}, cfg.Clusters)
	})

	t.Run("creates the config directory accessible by the user only", func(t *testing.T) {
		io, configFile, _ := testIO(t)

		_, err := io.read()

		require.NoError(t, err)
		assertPerm(t, filepath.Dir(configFile), 0o700)
	})

	t.Run("adds clusters only configured in connections.yaml", func(t *testing.T) {
		io, _, connectionsFile := testIO(t)
		writeFile(t, connectionsFile, `clusters:
    local:
        servers: [localhost:9092]
`, 0o600)

		cfg, err := io.read()

		require.NoError(t, err)
		require.Len(t, cfg.Clusters, 1)
		assert.Equal(t, "local", cfg.Clusters[0].Name)
		assert.Equal(t, []string{"localhost:9092"}, cfg.Clusters[0].BootstrapServers)
	})

	t.Run("refuses connection settings in config.yaml", func(t *testing.T) {
		io, configFile, _ := testIO(t)
		writeFile(t, configFile, `clusters:
    - name: prd
      servers: [localhost:9092]
      sasl:
          username: admin
          password: inline-secret
          authMethod: SASL_PLAINTEXT
`, 0o644)

		_, err := io.read()

		assert.ErrorContains(t, err, "config.yaml contains connection settings of cluster prd")
		assert.ErrorContains(t, err, "connections.yaml")
		assert.NotContains(t, err.Error(), "inline-secret")
		assert.Contains(t, readFile(t, configFile), "inline-secret", "config.yaml is left untouched")
	})

	t.Run("removes deleted and renamed clusters from connections.yaml", func(t *testing.T) {
		io, _, connectionsFile := testIO(t)
		cfg := &Config{Clusters: []Cluster{prdCluster(), {Name: "dev", SASLConfig: SASLConfig{Password: "dev-secret"}}}}
		cfg.ConfigIO = io
		require.NoError(t, io.write(cfg))

		cfg.DeleteCluster("dev")
		cfg.Clusters[0].Name = "prod"
		cfg.flush()

		connections := readFile(t, connectionsFile)
		assert.NotContains(t, connections, "dev")
		assert.NotContains(t, connections, "prd")
		assert.Contains(t, connections, "prod:")
	})

	t.Run("restricts existing connections files", func(t *testing.T) {
		io, _, connectionsFile := testIO(t)
		writeFile(t, connectionsFile, "{}", 0o644)

		require.NoError(t, io.write(&Config{Clusters: []Cluster{prdCluster()}}))

		assertPerm(t, connectionsFile, 0o600)
	})

	t.Run("does not create connections.yaml without clusters", func(t *testing.T) {
		io, _, connectionsFile := testIO(t)

		require.NoError(t, io.write(&Config{Theme: "nord"}))

		_, err := os.Stat(connectionsFile)
		assert.True(t, os.IsNotExist(err))
	})
}
