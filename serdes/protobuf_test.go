package serdes

import (
	"encoding/json"
	"ktea/config"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const commonProto = `syntax = "proto3";
package acme.common.v1;

message Money {
  string currency = 1;
  int64 amount = 2;
}
`

const ordersProto = `syntax = "proto3";
package acme.orders.v1;

import "acme/common/v1/money.proto";
import "google/protobuf/timestamp.proto";

message OrderCreated {
  string id = 1;
  acme.common.v1.Money total = 2;
  google.protobuf.Timestamp created_at = 3;
  map<string, string> labels = 4;

  message Line {
    string sku = 1;
  }
  repeated Line lines = 5;
}

message OrderCancelled {
  string id = 1;
  string reason = 2;
}
`

func requireProtoc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc is not installed")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func protoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "acme/common/v1/money.proto"), commonProto)
	writeFile(t, filepath.Join(dir, "acme/orders/v1/orders.proto"), ordersProto)
	// hidden directories are ignored
	writeFile(t, filepath.Join(dir, ".git/broken.proto"), "this is not a proto file")
	return dir
}

func compile(t *testing.T, dir string) *ProtoSchema {
	t.Helper()
	schema, err := CompileProtoDir("protoc", dir)
	require.NoError(t, err)
	return schema
}

func encode(t *testing.T, schema *ProtoSchema, messageType string, fields map[string]any) []byte {
	t.Helper()
	desc, err := schema.files.FindDescriptorByName(protoreflect.FullName(messageType))
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))
	for name, value := range fields {
		msg.Set(msg.Descriptor().Fields().ByName(protoreflect.Name(name)), protoreflect.ValueOf(value))
	}
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	return data
}

func asMap(t *testing.T, data DesData) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(data.Value), &m))
	return m
}

func TestCompileProtoDir(t *testing.T) {
	requireProtoc(t)

	t.Run("lists message types of the directory", func(t *testing.T) {
		schema := compile(t, protoDir(t))

		assert.Equal(t, []string{
			"acme.common.v1.Money",
			"acme.orders.v1.OrderCancelled",
			"acme.orders.v1.OrderCreated",
			"acme.orders.v1.OrderCreated.Line",
		}, schema.MessageTypes)
	})

	t.Run("decodes messages to json", func(t *testing.T) {
		schema := compile(t, protoDir(t))
		data := encode(t, schema, "acme.orders.v1.OrderCancelled", map[string]any{
			"id":     "o-1",
			"reason": "out of stock",
		})

		decoded, err := schema.Decode("acme.orders.v1.OrderCancelled", data)

		require.NoError(t, err)
		assert.Equal(t, "acme.orders.v1.OrderCancelled", decoded.MessageType)
		assert.Equal(t, map[string]any{"id": "o-1", "reason": "out of stock"}, asMap(t, decoded))
	})

	t.Run("fails for unknown message types", func(t *testing.T) {
		schema := compile(t, protoDir(t))

		_, err := schema.Decode("acme.Unknown", []byte{})

		assert.ErrorContains(t, err, "acme.Unknown not found")
	})

	t.Run("reports protoc errors", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "broken.proto"), `syntax = "proto3"; message {`)

		_, err := CompileProtoDir("protoc", dir)

		assert.ErrorContains(t, err, "protoc failed: broken.proto")
	})

	t.Run("reports missing protoc", func(t *testing.T) {
		_, err := CompileProtoDir("ktea-no-such-protoc", protoDir(t))

		assert.ErrorContains(t, err, "not found, install protoc")
	})

	t.Run("reports directories without proto files", func(t *testing.T) {
		_, err := CompileProtoDir("protoc", t.TempDir())

		assert.ErrorContains(t, err, "no .proto files found")
	})
}

func TestProtobufDecoders(t *testing.T) {
	requireProtoc(t)

	dir := protoDir(t)
	schema := compile(t, dir)
	created := encode(t, schema, "acme.orders.v1.OrderCancelled", map[string]any{"id": "o-1"})

	topicCfg := config.ProtobufTopicConfig{
		Topic:    "orders",
		ProtoDir: dir,
		Rules: []config.ProtobufRule{
			{MessageType: "acme.orders.v1.OrderCancelled", Header: "type", ValueRegex: "^Order(Cancelled|Deleted)$"},
			{MessageType: "acme.orders.v1.OrderCreated"},
		},
	}

	t.Run("uses the first matching header rule", func(t *testing.T) {
		d := NewProtobufDecoders(&config.ProtobufConfig{Topics: []config.ProtobufTopicConfig{topicCfg}})

		data, handled, err := d.Decode("orders", []Header{{"type", []byte("OrderCancelled")}}, created)

		require.NoError(t, err)
		assert.True(t, handled)
		assert.Equal(t, "acme.orders.v1.OrderCancelled", data.MessageType)
	})

	t.Run("uses the fallback rule when no header rule matches", func(t *testing.T) {
		d := NewProtobufDecoders(&config.ProtobufConfig{Topics: []config.ProtobufTopicConfig{topicCfg}})

		data, handled, err := d.Decode("orders", []Header{{"type", []byte("OrderCreated")}}, created)

		require.NoError(t, err)
		assert.True(t, handled)
		assert.Equal(t, "acme.orders.v1.OrderCreated", data.MessageType)
	})

	t.Run("does not handle records without matching rule", func(t *testing.T) {
		cfg := topicCfg
		cfg.Rules = cfg.Rules[:1]
		d := NewProtobufDecoders(&config.ProtobufConfig{Topics: []config.ProtobufTopicConfig{cfg}})

		_, handled, err := d.Decode("orders", nil, created)

		require.NoError(t, err)
		assert.False(t, handled)
	})

	t.Run("does not handle unknown topics", func(t *testing.T) {
		d := NewProtobufDecoders(nil)

		_, handled, err := d.Decode("orders", nil, created)

		require.NoError(t, err)
		assert.False(t, handled)
	})

	t.Run("compiles lazily once and reports compile errors", func(t *testing.T) {
		calls := 0
		d := newProtobufDecoders(
			&config.ProtobufConfig{Protoc: "my-protoc", Topics: []config.ProtobufTopicConfig{topicCfg}},
			func(protoc, dir string) (*ProtoSchema, error) {
				calls++
				assert.Equal(t, "my-protoc", protoc)
				assert.Equal(t, topicCfg.ProtoDir, dir)
				return nil, assert.AnError
			},
		)

		_, handled, err1 := d.Decode("orders", nil, created)
		_, _, err2 := d.Decode("orders", nil, created)

		assert.True(t, handled)
		assert.ErrorIs(t, err1, assert.AnError)
		assert.ErrorIs(t, err2, assert.AnError)
		assert.Equal(t, 1, calls)
	})

	t.Run("reports invalid regexes", func(t *testing.T) {
		cfg := topicCfg
		cfg.Rules = []config.ProtobufRule{{MessageType: "x", Header: "type", ValueRegex: "("}}
		d := NewProtobufDecoders(&config.ProtobufConfig{Topics: []config.ProtobufTopicConfig{cfg}})

		_, handled, err := d.Decode("orders", nil, created)

		assert.True(t, handled)
		assert.ErrorContains(t, err, "invalid protobuf value regex")
	})

	t.Run("set and delete topics", func(t *testing.T) {
		d := NewProtobufDecoders(nil)

		d.SetTopic(topicCfg, schema)
		_, handled, err := d.Decode("orders", nil, created)
		require.NoError(t, err)
		assert.True(t, handled)
		assert.Equal(t, uint64(1), d.Version())

		d.DeleteTopic("orders")
		_, handled, _ = d.Decode("orders", nil, created)
		assert.False(t, handled)
		assert.Equal(t, uint64(2), d.Version())
	})
}
