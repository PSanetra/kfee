package ccloud

import (
	"bytes"
	"errors"
	"ktea/config"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCLI answers Confluent CLI calls by their arguments and records them.
type fakeCLI struct {
	responses map[string]string
	errs      map[string]error
	calls     []string
}

func (f *fakeCLI) run(args ...string) ([]byte, error) {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	for prefix, err := range f.errs {
		if strings.HasPrefix(call, prefix) {
			return nil, err
		}
	}
	for prefix, out := range f.responses {
		if strings.HasPrefix(call, prefix) {
			return []byte(out), nil
		}
	}
	return nil, errors.New("unexpected call: " + call)
}

func TestCLI(t *testing.T) {
	fake := &fakeCLI{responses: map[string]string{
		"environment list": `[{"is_current": true, "id": "env-1", "name": "prod", "stream_governance_package": "ESSENTIALS"}]`,
		"kafka cluster list": `[{"is_current": false, "id": "lkc-1", "name": "orders", "type": "BASIC", "cloud": "aws",
			"region": "eu-central-1", "status": "UP", "endpoint": "SASL_SSL://pkc-1.eu-central-1.aws.confluent.cloud:9092"}]`,
		"schema-registry cluster describe": `{"name": "sr", "cluster": "lsrc-1", "endpoint_url": "https://psrc-1.eu-central-1.aws.confluent.cloud"}`,
		"api-key create":                   `{"api_key": "KEY", "api_secret": "SECRET"}`,
	}}
	cli := NewCLI(fake.run)

	t.Run("lists environments", func(t *testing.T) {
		envs, err := cli.Environments()

		require.NoError(t, err)
		assert.Equal(t, []Environment{{ID: "env-1", Name: "prod", IsCurrent: true}}, envs)
		assert.Equal(t, "environment list --output json", fake.calls[len(fake.calls)-1])
	})

	t.Run("lists kafka clusters", func(t *testing.T) {
		clusters, err := cli.KafkaClusters("env-1")

		require.NoError(t, err)
		require.Len(t, clusters, 1)
		assert.Equal(t, "lkc-1", clusters[0].ID)
		assert.Equal(t, "pkc-1.eu-central-1.aws.confluent.cloud:9092", clusters[0].BootstrapServer())
		assert.Equal(t, "kafka cluster list --environment env-1 --output json", fake.calls[len(fake.calls)-1])
	})

	t.Run("describes the schema registry", func(t *testing.T) {
		sr, err := cli.SchemaRegistry("env-1")

		require.NoError(t, err)
		assert.Equal(t, SchemaRegistry{ID: "lsrc-1", EndpointURL: "https://psrc-1.eu-central-1.aws.confluent.cloud"}, sr)
	})

	t.Run("creates api keys", func(t *testing.T) {
		key, err := cli.CreateAPIKey("env-1", "lkc-1", "ktea 2026-11-18 14:05:07")

		require.NoError(t, err)
		assert.Equal(t, APIKey{Key: "KEY", Secret: "SECRET"}, key)
		assert.Equal(t, "api-key create --resource lkc-1 --environment env-1 --description ktea 2026-11-18 14:05:07 --output json",
			fake.calls[len(fake.calls)-1])
	})

	t.Run("reports environments without schema registry", func(t *testing.T) {
		_, err := NewCLI((&fakeCLI{responses: map[string]string{"schema-registry": `{}`}}).run).SchemaRegistry("env-1")

		assert.ErrorContains(t, err, "no Schema Registry")
	})

	t.Run("reports unexpected output", func(t *testing.T) {
		_, err := NewCLI((&fakeCLI{responses: map[string]string{"environment": `Error: expired token`}}).run).Environments()

		assert.ErrorContains(t, err, "unexpected output of confluent environment list")
	})

	t.Run("skips notices before the json", func(t *testing.T) {
		envs, err := NewCLI((&fakeCLI{responses: map[string]string{
			"environment": "A minor version update is available.\n\n[{\"id\": \"env-1\"}]",
		}}).run).Environments()

		require.NoError(t, err)
		assert.Equal(t, []Environment{{ID: "env-1"}}, envs)
	})
}

func TestKeyDescription(t *testing.T) {
	assert.Equal(t, "ktea 2026-11-18 14:05:07", KeyDescription(time.Date(2026, 11, 18, 14, 5, 7, 0, time.UTC)))
}

func TestWizardApply(t *testing.T) {
	now := time.Date(2026, 11, 18, 14, 5, 7, 0, time.UTC)
	sr := &SchemaRegistry{ID: "lsrc-1", EndpointURL: "https://psrc-1.confluent.cloud"}
	s := setup{
		env:      Environment{ID: "env-1", Name: "prod"},
		cluster:  KafkaCluster{ID: "lkc-1", Name: "orders", Endpoint: "SASL_SSL://pkc-1.confluent.cloud:9092"},
		name:     "orders-prod",
		color:    "#00FF00",
		sr:       sr,
		activate: true,
	}
	newWizard := func(fake *fakeCLI, cfg *config.Config) (*Wizard, *bytes.Buffer) {
		config.NewInMemoryConfigIO(cfg)
		out := &bytes.Buffer{}
		return &Wizard{cli: NewCLI(fake.run), cfg: cfg, now: func() time.Time { return now }, out: out}, out
	}
	keys := func() *fakeCLI {
		return &fakeCLI{responses: map[string]string{
			"api-key create --resource lkc-1":  `{"api_key": "KAFKA-KEY", "api_secret": "KAFKA-SECRET"}`,
			"api-key create --resource lsrc-1": `{"api_key": "SR-KEY", "api_secret": "SR-SECRET"}`,
		}}
	}

	t.Run("creates api keys and adds the active cluster", func(t *testing.T) {
		fake := keys()
		cfg := &config.Config{Clusters: []config.Cluster{{Name: "local", Active: true}}}
		w, out := newWizard(fake, cfg)

		require.NoError(t, w.apply(s))

		assert.Equal(t, []string{
			"api-key create --resource lkc-1 --environment env-1 --description ktea 2026-11-18 14:05:07 --output json",
			"api-key create --resource lsrc-1 --environment env-1 --description ktea 2026-11-18 14:05:07 --output json",
		}, fake.calls)
		cluster := cfg.FindClusterByName("orders-prod")
		require.NotNil(t, cluster)
		assert.Equal(t, config.Cluster{
			Name:             "orders-prod",
			Color:            "#00FF00",
			Active:           true,
			BootstrapServers: []string{"pkc-1.confluent.cloud:9092"},
			SASLConfig: config.SASLConfig{
				Username:   "KAFKA-KEY",
				Password:   "KAFKA-SECRET",
				AuthMethod: config.AuthMethodSASLPlaintext,
			},
			SchemaRegistry: &config.SchemaRegistryConfig{
				Url:      "https://psrc-1.confluent.cloud",
				Username: "SR-KEY",
				Password: "SR-SECRET",
			},
			TLSConfig: config.TLSConfig{Enable: true},
		}, *cluster)
		assert.False(t, cfg.FindClusterByName("local").Active)
		assert.NotContains(t, out.String(), "SECRET")
	})

	t.Run("updates an existing cluster keeping its kafka connect clusters", func(t *testing.T) {
		cfg := &config.Config{Clusters: []config.Cluster{{
			Name:                 "orders-prod",
			Active:               true,
			KafkaConnectClusters: []config.KafkaConnectConfig{{Name: "connect", Url: "https://connect"}},
		}}}
		w, _ := newWizard(keys(), cfg)
		noSR := s
		noSR.sr = nil

		require.NoError(t, w.apply(noSR))

		require.Len(t, cfg.Clusters, 1)
		assert.Equal(t, "KAFKA-KEY", cfg.Clusters[0].SASLConfig.Username)
		assert.Nil(t, cfg.Clusters[0].SchemaRegistry)
		assert.Equal(t, []config.KafkaConnectConfig{{Name: "connect", Url: "https://connect"}}, cfg.Clusters[0].KafkaConnectClusters)
	})

	t.Run("skips the schema registry when its api key can not be created", func(t *testing.T) {
		fake := keys()
		fake.errs = map[string]error{"api-key create --resource lsrc-1": errors.New("forbidden")}
		cfg := &config.Config{}
		w, out := newWizard(fake, cfg)

		require.NoError(t, w.apply(s))

		assert.Nil(t, cfg.FindClusterByName("orders-prod").SchemaRegistry)
		assert.Contains(t, out.String(), "Skipping Schema Registry: forbidden")
	})

	t.Run("does not change the config when the kafka api key can not be created", func(t *testing.T) {
		fake := keys()
		fake.errs = map[string]error{"api-key create --resource lkc-1": errors.New("forbidden")}
		cfg := &config.Config{}
		w, _ := newWizard(fake, cfg)

		assert.ErrorContains(t, w.apply(s), "forbidden")
		assert.Empty(t, cfg.Clusters)
	})
}
