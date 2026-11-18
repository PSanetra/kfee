package record_details_page

import (
	"errors"
	"ktea/config"
	"ktea/kadmin"
	"ktea/serdes"
	"ktea/tests"
	"ktea/ui/clipper"
	"ktea/ui/components/statusbar"
	"ktea/ui/tabs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ordersProto = `syntax = "proto3";
package acme.orders.v1;

message OrderCreated {
  string id = 1;
  int64 amount = 2;
}

message OrderCancelled {
  string id = 1;
}
`

// orderCancelled is acme.orders.v1.OrderCancelled{id: "o-1"} in protobuf wire format.
var orderCancelled = []byte{0x0a, 0x03, 'o', '-', '1'}

func protoRecord() *kadmin.ConsumerRecord {
	return &kadmin.ConsumerRecord{
		Payload:  serdes.DesData{Value: string(orderCancelled)},
		RawValue: orderCancelled,
		Headers: []kadmin.Header{
			{Key: "source", Value: kadmin.NewHeaderValue("shop")},
			{Key: "type", Value: kadmin.NewHeaderValue("OrderCancelled")},
		},
	}
}

func fakeSchema(dir string) *serdes.ProtoSchema {
	return &serdes.ProtoSchema{
		Dir:          dir,
		MessageTypes: []string{"acme.orders.v1.OrderCancelled", "acme.orders.v1.OrderCreated"},
	}
}

// send passes msg to the modal and feeds the resulting messages back, except the ones closing it.
func send(m *protoDecoderModal, msg tea.Msg) []tea.Msg {
	var result []tea.Msg
	for _, msg := range tests.ExecuteBatchCmd(m.Update(msg)) {
		switch msg.(type) {
		case protoSchemaCompiledMsg:
			result = append(result, send(m, msg)...)
		default:
			result = append(result, msg)
		}
	}
	return result
}

func newModal(t *testing.T, cfg *config.ProtobufConfig, compile serdes.ProtoCompiler) *protoDecoderModal {
	t.Helper()
	return newProtoDecoderModal("orders", protoRecord(), cfg, "protoc", compile, "")
}

func TestProtoDecoderModal(t *testing.T) {
	enter := tests.Key(tea.KeyEnter)
	down := tests.Key(tea.KeyDown)

	dirCfg := func(dir string) *config.ProtobufConfig {
		// the proto directory of another topic is prefilled
		return &config.ProtobufConfig{Topics: []config.ProtobufTopicConfig{{Topic: "other", ProtoDir: dir}}}
	}
	compileOk := func(protoc, dir string) (*serdes.ProtoSchema, error) {
		return fakeSchema(dir), nil
	}

	t.Run("configures a header rule", func(t *testing.T) {
		dir := t.TempDir()
		m := newModal(t, dirCfg(dir), compileOk)
		assert.Contains(t, ansi.Strip(m.View(100)), "Proto directory")

		send(m, enter)
		assert.Equal(t, stageMessageType, m.stage)
		assert.Contains(t, ansi.Strip(m.View(100)), "acme.orders.v1.OrderCreated")

		send(m, enter)
		assert.Equal(t, stageHeader, m.stage)
		assert.Contains(t, ansi.Strip(m.View(100)), "type = OrderCancelled")

		send(m, down)
		send(m, down)
		send(m, enter)
		assert.Equal(t, stageRegex, m.stage)
		assert.Equal(t, "^OrderCancelled$", m.values.regex)

		msgs := send(m, enter)

		assert.Equal(t, []tea.Msg{protoDecoderSavedMsg{
			topic: config.ProtobufTopicConfig{
				Topic:    "orders",
				ProtoDir: dir,
				Rules: []config.ProtobufRule{{
					MessageType: "acme.orders.v1.OrderCancelled",
					Header:      "type",
					ValueRegex:  "^OrderCancelled$",
				}},
			},
			schema: fakeSchema(dir),
		}}, msgs)
	})

	t.Run("configures a fallback rule", func(t *testing.T) {
		dir := t.TempDir()
		m := newModal(t, dirCfg(dir), compileOk)

		send(m, enter)
		send(m, down)
		send(m, enter)
		msgs := send(m, enter)

		require.Len(t, msgs, 1)
		assert.Equal(t, []config.ProtobufRule{{MessageType: "acme.orders.v1.OrderCreated"}},
			msgs[0].(protoDecoderSavedMsg).topic.Rules)
	})

	t.Run("rejects regexes not matching the record", func(t *testing.T) {
		m := newModal(t, dirCfg(t.TempDir()), compileOk)
		send(m, enter)
		send(m, enter)
		send(m, down)
		send(m, down)
		send(m, enter)

		send(m, tests.Key('x'))
		msgs := send(m, enter)

		assert.Empty(t, msgs)
		assert.Equal(t, stageRegex, m.stage)
		assert.Contains(t, ansi.Strip(m.View(100)), "does not match the type header")
	})

	t.Run("shows compile errors", func(t *testing.T) {
		m := newModal(t, dirCfg(t.TempDir()), func(protoc, dir string) (*serdes.ProtoSchema, error) {
			return nil, errors.New("protoc failed: orders.proto:1:1: Syntax error")
		})

		send(m, enter)

		assert.Equal(t, stageDir, m.stage)
		assert.Contains(t, ansi.Strip(m.View(100)), "Syntax error")
	})

	t.Run("rejects missing directories", func(t *testing.T) {
		m := newModal(t, dirCfg(filepath.Join(t.TempDir(), "missing")), compileOk)

		send(m, enter)

		assert.Equal(t, stageDir, m.stage)
		assert.Contains(t, m.err, "is not a directory")
	})

	t.Run("removes an existing decoder", func(t *testing.T) {
		m := newModal(t, &config.ProtobufConfig{Topics: []config.ProtobufTopicConfig{{Topic: "orders"}}}, compileOk)
		assert.Equal(t, stageAction, m.stage)

		send(m, down)
		msgs := send(m, enter)

		assert.Equal(t, []tea.Msg{protoDecoderRemovedMsg{topic: "orders"}}, msgs)
	})

	t.Run("esc closes, except while filtering", func(t *testing.T) {
		m := newModal(t, dirCfg(t.TempDir()), compileOk)
		send(m, enter)

		send(m, tests.Key('/'))
		assert.Empty(t, send(m, tests.Key(tea.KeyEscape)))

		assert.Equal(t, []tea.Msg{protoDecoderClosedMsg{}}, send(m, tests.Key(tea.KeyEscape)))
	})
}

func TestRecordDetailsPageProtobuf(t *testing.T) {
	newPage := func(t *testing.T) (*Model, *config.Config) {
		t.Helper()
		cfg := &config.Config{Clusters: []config.Cluster{{Name: "prd", Active: true}}}
		config.NewInMemoryConfigIO(cfg)
		ktx := tests.NewKontext(tests.WithConfig(cfg))
		ktx.SetProtoDecoders(serdes.NewProtobufDecoders(nil))
		record := protoRecord()
		m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
		m.View(tests.Kontext, tests.Renderer)
		return m, cfg
	}

	t.Run("p opens the protobuf decoder modal", func(t *testing.T) {
		m, _ := newPage(t)
		assert.Contains(t, m.Shortcuts(), statusbar.Shortcut{Name: "Protobuf Decoder", Keybinding: "p"})

		m.Update(tests.Key('p'))

		assert.NotNil(t, m.protoModal)
		assert.Contains(t, ansi.Strip(m.View(tests.Kontext, tests.Renderer)), "Protobuf decoder for orders")
	})

	t.Run("p preselects the highlighted header", func(t *testing.T) {
		m, _ := newPage(t)
		m.Update(tests.Key('h'))
		m.Update(tests.Key(tea.KeyDown))

		m.Update(tests.Key('p'))

		assert.Equal(t, "type", m.protoModal.values.header)
	})

	t.Run("saving decodes the records and stores the decoder", func(t *testing.T) {
		if _, err := exec.LookPath("protoc"); err != nil {
			t.Skip("protoc is not installed")
		}
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "orders.proto"), []byte(ordersProto), 0o644))
		schema, err := serdes.CompileProtoDir("protoc", dir)
		require.NoError(t, err)
		topicCfg := config.ProtobufTopicConfig{
			Topic:    "orders",
			ProtoDir: dir,
			Rules:    []config.ProtobufRule{{MessageType: "acme.orders.v1.OrderCancelled"}},
		}
		m, cfg := newPage(t)
		m.Update(tests.Key('p'))

		m.Update(protoDecoderSavedMsg{topic: topicCfg, schema: schema})

		assert.Nil(t, m.protoModal)
		assert.Equal(t, []config.ProtobufTopicConfig{topicCfg}, cfg.Clusters[0].Protobuf.Topics)
		assert.Equal(t, "acme.orders.v1.OrderCancelled", m.records[0].Payload.MessageType)
		render := ansi.Strip(m.View(tests.Kontext, tests.Renderer))
		assert.Contains(t, render, `"id": "o-1"`)
		assert.Contains(t, render, "[ Protobuf ]")
		assert.Contains(t, render, "type: acme.orders.v1.OrderCancelled")

		m.Update(protoDecoderRemovedMsg{topic: "orders"})

		assert.Empty(t, cfg.Clusters[0].Protobuf.Topics)
		assert.Equal(t, string(orderCancelled), m.records[0].Payload.Value)
		assert.Empty(t, m.records[0].Payload.MessageType)
	})
}
