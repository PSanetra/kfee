package serdes

import (
	"ktea/config"
	"math"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestDecodeRaw(t *testing.T) {
	var money []byte
	money = protowire.AppendTag(money, 1, protowire.BytesType)
	money = protowire.AppendString(money, "EUR")
	money = protowire.AppendTag(money, 2, protowire.VarintType)
	money = protowire.AppendVarint(money, 1200)

	var order []byte
	order = protowire.AppendTag(order, 1, protowire.BytesType)
	order = protowire.AppendString(order, "o-1")
	order = protowire.AppendTag(order, 2, protowire.BytesType)
	order = protowire.AppendBytes(order, money)
	for _, sku := range []string{"sku-1", "sku-2"} {
		var line []byte
		line = protowire.AppendTag(line, 1, protowire.BytesType)
		line = protowire.AppendString(line, sku)
		order = protowire.AppendTag(order, 3, protowire.BytesType)
		order = protowire.AppendBytes(order, line)
	}
	order = protowire.AppendTag(order, 5, protowire.VarintType)
	// -1 as int64, encoded as 10 byte varint
	order = protowire.AppendVarint(order, math.MaxUint64)
	order = protowire.AppendTag(order, 6, protowire.Fixed64Type)
	order = protowire.AppendFixed64(order, 42)
	order = protowire.AppendTag(order, 7, protowire.Fixed32Type)
	order = protowire.AppendFixed32(order, 7)
	order = protowire.AppendTag(order, 8, protowire.BytesType)
	order = protowire.AppendBytes(order, []byte{0xff, 0x00, 0xfe})
	order = protowire.AppendTag(order, 9, protowire.BytesType)
	order = protowire.AppendString(order, "")
	// fields are sorted by number
	order = protowire.AppendTag(order, 4, protowire.VarintType)
	order = protowire.AppendVarint(order, 1)

	t.Run("decodes fields by number", func(t *testing.T) {
		data, err := DecodeRaw(order)

		require.NoError(t, err)
		assert.Equal(t, RawMessageType, data.MessageType)
		assert.JSONEq(t, `{
			"1": "o-1",
			"2": {"1": "EUR", "2": 1200},
			"3": [{"1": "sku-1"}, {"1": "sku-2"}],
			"4": 1,
			"5": -1,
			"6": 42,
			"7": 7,
			"8": "0xff00fe",
			"9": ""
		}`, data.Value)
		assert.True(t, strings.Index(data.Value, `"4"`) < strings.Index(data.Value, `"5"`))
	})

	t.Run("fails for data that is not protobuf", func(t *testing.T) {
		_, err := DecodeRaw([]byte(`{"json": true}`))

		assert.ErrorContains(t, err, "unable to decode record as protobuf")
	})

	t.Run("fails for truncated data", func(t *testing.T) {
		_, err := DecodeRaw(order[:len(order)-5])

		assert.Error(t, err)
	})

	t.Run("is consistent with protoc --decode_raw", func(t *testing.T) {
		if _, err := exec.LookPath("protoc"); err != nil {
			t.Skip("protoc is not installed")
		}
		cmd := exec.Command("protoc", "--decode_raw")
		cmd.Stdin = strings.NewReader(string(money))
		out, err := cmd.Output()
		require.NoError(t, err)
		assert.Equal(t, "1: \"EUR\"\n2: 1200\n", string(out))

		data, err := DecodeRaw(money)

		require.NoError(t, err)
		assert.JSONEq(t, `{"1": "EUR", "2": 1200}`, data.Value)
	})
}

func TestProtobufDecodersRaw(t *testing.T) {
	var value []byte
	value = protowire.AppendTag(value, 1, protowire.BytesType)
	value = protowire.AppendString(value, "o-1")
	compiled := false
	d := newProtobufDecoders(nil, func(string, string) (*ProtoSchema, error) {
		compiled = true
		return nil, assert.AnError
	})
	d.SetTopic(config.ProtobufTopicConfig{
		Topic:    "orders",
		ProtoDir: "/protos",
		Rules:    []config.ProtobufRule{{Raw: true}},
	}, nil)

	data, handled, err := d.Decode("orders", nil, value)

	require.NoError(t, err)
	assert.True(t, handled)
	assert.JSONEq(t, `{"1": "o-1"}`, data.Value)
	assert.False(t, compiled, "raw rules do not compile the proto directory")
}
