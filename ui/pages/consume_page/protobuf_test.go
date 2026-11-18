package consume_page

import (
	"ktea/config"
	"ktea/kadmin"
	"ktea/serdes"
	"ktea/ui"
	"ktea/ui/tabs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumptionPageRedecodesProtobuf(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc is not installed")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "order.proto"), []byte(
		`syntax = "proto3"; package acme; message Order { string id = 1; }`), 0o644))
	schema, err := serdes.CompileProtoDir("protoc", dir)
	require.NoError(t, err)

	// acme.Order{id: "o-1"}
	value := []byte{0x0a, 0x03, 'o', '-', '1'}
	decoders := serdes.NewProtobufDecoders(nil)
	topic := &kadmin.ListedTopic{Name: "orders", PartitionCount: 1}
	page, _ := New(kadmin.NewMockKadmin(), decoders, kadmin.NewDefaultReadDetails(topic), topic, false, tabs.NewMockTopicsTabNavigator())
	m := page.(*Model)
	m.Update(kadmin.ConsumerRecordReceived{Records: []kadmin.ConsumerRecord{
		{Payload: serdes.DesData{Value: string(value)}, RawValue: value},
	}})

	t.Run("keeps the records while the decoders did not change", func(t *testing.T) {
		m.Update(ui.RegainedFocusMsg{})

		assert.Equal(t, string(value), m.records[0].Payload.Value)
	})

	t.Run("decodes the records again after the decoders changed", func(t *testing.T) {
		decoders.SetTopic(config.ProtobufTopicConfig{
			Topic:    "orders",
			ProtoDir: dir,
			Rules:    []config.ProtobufRule{{MessageType: "acme.Order"}},
		}, schema)

		m.Update(ui.RegainedFocusMsg{})

		assert.Equal(t, "acme.Order", m.records[0].Payload.MessageType)
		assert.JSONEq(t, `{"id": "o-1"}`, m.records[0].Payload.Value)
	})
}
