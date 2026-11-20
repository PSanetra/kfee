package consume_page

import (
	"kfee/serdes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecoderName(t *testing.T) {
	assert.Equal(t, "orders-value", decoderName(serdes.DesData{RuleID: "avro:orders-value", SchemaID: 7}),
		"the Avro subject")
	assert.Equal(t, "OrderUpsertMessageV1-value", decoderName(serdes.DesData{
		RuleID: "avro:com.example.orders.events.OrderUpsertMessageV1-value",
	}), "dotted subjects show their last part")
	assert.Equal(t, "OrderCancelled", decoderName(serdes.DesData{
		RuleID:      "3e45d3ba5fb994af",
		MessageType: "acme.orders.v1.OrderCancelled",
	}), "the last part of the message type of the protobuf rule")
	assert.Equal(t, "without schema", decoderName(serdes.DesData{
		RuleID:      "3e45d3ba5fb994af",
		MessageType: serdes.RawMessageType,
	}))
	assert.Empty(t, decoderName(serdes.DesData{}), "not decoded")
}
