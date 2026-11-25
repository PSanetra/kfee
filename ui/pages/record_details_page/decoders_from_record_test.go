package record_details_page

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kfee/kadmin"
	"kfee/serdes"
	"kfee/tests"
	"kfee/ui/clipper"
	"kfee/ui/tabs"
)

func TestDecodersFromRecord(t *testing.T) {
	t.Run("d opens the decoders of the topic with the schema of an Avro record, back returns to the record", func(t *testing.T) {
		record := &kadmin.ConsumerRecord{
			Payload: serdes.DesData{Value: "{}", RuleID: "avro:orders-value", Schema: `{"type":"record"}`},
		}
		ktx := tests.NewKontext()
		navigator := tabs.NewMockTopicsTabNavigator()
		m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, navigator)
		m.View(ktx, tests.Renderer)

		cmd := m.Update(tests.Key('d'))
		require.NotNil(t, cmd)
		msg, ok := cmd().(tabs.ToDecodersPageCalledMsg)
		require.True(t, ok)
		assert.Equal(t, "orders", msg.Details.Topic)
		assert.Equal(t, "avro:orders-value", msg.Details.Decoder)
		assert.Equal(t, map[string]string{"avro:orders-value": `{"type":"record"}`}, msg.Details.AvroSchemas)

		back, ok := msg.Details.Back()().(tabs.ToRecordDetailsPageCalledMsg)
		require.True(t, ok)
		assert.Equal(t, "orders", back.Details.TopicName)
		assert.Equal(t, record, back.Details.Record)
	})

	t.Run("d passes the decoder of the record, without a schema for protobuf", func(t *testing.T) {
		record := &kadmin.ConsumerRecord{Payload: serdes.DesData{Value: "{}", RuleID: "cancelled"}}
		ktx := tests.NewKontext()
		m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
		m.View(ktx, tests.Renderer)

		msg, ok := m.Update(tests.Key('d'))().(tabs.ToDecodersPageCalledMsg)
		require.True(t, ok)
		assert.Equal(t, "cancelled", msg.Details.Decoder)
		assert.Empty(t, msg.Details.AvroSchemas)
	})

	t.Run("d does nothing for a record that is not decoded", func(t *testing.T) {
		record := &kadmin.ConsumerRecord{Payload: serdes.DesData{Value: "{}"}}
		ktx := tests.NewKontext()
		m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
		m.View(ktx, tests.Renderer)

		assert.Nil(t, m.Update(tests.Key('d')))
	})
}
