package record_details_page

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"kfee/kadmin"
	"kfee/serdes"
	"kfee/tests"
	"kfee/ui/clipper"
	"kfee/ui/tabs"
)

func TestProtobufDecoderIsNotOfferedForAvroRecords(t *testing.T) {
	t.Run("p does nothing and the shortcut is hidden for an Avro record", func(t *testing.T) {
		record := &kadmin.ConsumerRecord{
			Payload: serdes.DesData{Value: "{}", RuleID: "avro:orders-value"},
		}
		ktx := tests.NewKontext(tests.WithWindowWidth(120))
		m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
		m.View(ktx, tests.Renderer)

		m.Update(tests.Key('p'))

		assert.Nil(t, m.protoModal)
		for _, s := range m.Shortcuts() {
			assert.NotEqual(t, "Protobuf Decoder", s.Name)
		}
	})

	t.Run("p opens the decoder for a protobuf record", func(t *testing.T) {
		record := &kadmin.ConsumerRecord{
			Payload: serdes.DesData{Value: "{}"},
		}
		ktx := tests.NewKontext(tests.WithWindowWidth(120))
		m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
		m.View(ktx, tests.Renderer)

		m.Update(tests.Key('p'))
		assert.NotNil(t, m.protoModal)
	})
}
