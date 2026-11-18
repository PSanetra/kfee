package kadmin

import (
	"context"
	"kfee/config"
	"kfee/serdes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadProtobufRecords(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc is not installed")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "orders.proto"), []byte(`syntax = "proto3";
package acme;
message OrderCreated { string id = 1; }
message OrderCancelled { string id = 1; string reason = 2; }
`), 0o644))

	topic := topicName()
	msg := ka.CreateTopic(TopicCreationDetails{Name: topic, NumPartitions: 1, ReplicationFactor: 1}).(TopicCreationStartedMsg)
	if _, ok := msg.AwaitCompletion().(TopicCreatedMsg); !ok {
		t.Fatal("Unable to create topic")
	}
	defer ka.DeleteTopic(topic)

	// OrderCreated{id: "o-1"} and OrderCancelled{id: "o-1", reason: "late"}
	created := []byte{0x0a, 0x03, 'o', '-', '1'}
	cancelled := []byte{0x0a, 0x03, 'o', '-', '1', 0x12, 0x04, 'l', 'a', 't', 'e'}
	for _, r := range []*ProducerRecord{
		{Topic: topic, Key: "1", Value: created},
		{Topic: topic, Key: "2", Value: cancelled, Headers: map[string]string{"type": "OrderCancelled"}},
	} {
		psm := ka.PublishRecord(r)
		select {
		case err := <-psm.Err:
			t.Fatal("Unable to publish", err)
		case <-psm.Published:
		}
	}

	topicCfg := config.ProtobufTopicConfig{
		Topic:    topic,
		ProtoDir: dir,
		Rules: []config.ProtobufRule{
			{MessageType: "acme.OrderCancelled", Header: "type", ValueRegex: "^OrderCancelled$"},
			{MessageType: "acme.OrderCreated"},
		},
	}
	decoders := serdes.NewProtobufDecoders(&config.ProtobufConfig{Topics: []config.ProtobufTopicConfig{topicCfg}})
	ka.SetProtoDecoders(decoders)
	defer ka.SetProtoDecoders(nil)

	read := func(filter *Filter) map[string]ConsumerRecord {
		rsm := ka.ReadRecords(context.Background(), ReadDetails{
			TopicName:       topic,
			PartitionToRead: []int{0},
			StartPoint:      Beginning,
			Limit:           50,
			Filter:          filter,
		}).(*ReadingStartedMsg)
		records := map[string]ConsumerRecord{}
		timeout := time.After(10 * time.Second)
		for {
			select {
			case r, ok := <-rsm.ConsumerRecord:
				if !ok {
					return records
				}
				records[r.Key] = r
			case <-timeout:
				t.Fatal("timed out reading records")
			}
		}
	}

	t.Run("decodes records by header rule and fallback", func(t *testing.T) {
		records := read(nil)

		require.Len(t, records, 2)
		require.NoError(t, records["1"].Err)
		assert.Equal(t, "acme.OrderCreated", records["1"].Payload.MessageType)
		assert.JSONEq(t, `{"id": "o-1"}`, records["1"].Payload.Value)
		created := records["1"]
		assert.Equal(t, "Protobuf", created.PayloadType())
		require.NoError(t, records["2"].Err)
		assert.Equal(t, "acme.OrderCancelled", records["2"].Payload.MessageType)
		assert.JSONEq(t, `{"id": "o-1", "reason": "late"}`, records["2"].Payload.Value)
		assert.Equal(t, cancelled, records["2"].RawValue)
	})

	t.Run("filters on the decoded value", func(t *testing.T) {
		records := read(&Filter{Term: "late"})

		assert.Len(t, records, 1)
		assert.Contains(t, records, "2")
	})

	t.Run("redecodes records after the decoders changed", func(t *testing.T) {
		record := read(nil)["2"]

		decoders.DeleteTopic(topic)
		record = record.RedecodeProtobuf(decoders, topic)
		assert.Equal(t, string(cancelled), record.Payload.Value)
		assert.Empty(t, record.Payload.MessageType)

		decoders.SetTopic(topicCfg, nil)
		record = record.RedecodeProtobuf(decoders, topic)
		assert.Equal(t, "acme.OrderCancelled", record.Payload.MessageType)
	})
}
