package kadmin

import (
	"context"
	"time"

	"charm.land/log/v2"
	"github.com/IBM/sarama"
)

// searchBackwards searches a partition from the most recent record backwards, so the most recent
// matches are found first. As Kafka only reads forward, the partition is read in ranges, each one
// before the previous one, growing from searchInitialChunk to searchMaxChunk records. The matches
// of a range are sent newest first, as long as send does not ask to stop. limitReached stops the
// search when other partitions found enough matches.
func (ka *SaramaKafkaAdmin) searchBackwards(
	ctx context.Context,
	client sarama.Consumer,
	topic string,
	partition int32,
	off offsets,
	match func(*sarama.ConsumerMessage) (ConsumerRecord, bool),
	send func(ConsumerRecord) (stop bool),
	limitReached func() bool,
) error {
	if off.start < 0 {
		return nil
	}
	hi := off.newest()
	chunk := searchInitialChunk
	for hi >= off.start {
		lo := max(off.start, hi-chunk+1)
		log.Debug("Searching range", "topic", topic, "partition", partition, "from", lo, "to", hi)

		var matches []ConsumerRecord
		err := consumeRange(ctx, client, topic, partition, lo, hi, func(msg *sarama.ConsumerMessage) bool {
			if record, ok := match(msg); ok {
				matches = append(matches, record)
			}
			return limitReached()
		})
		if err != nil {
			return err
		}
		for i := len(matches) - 1; i >= 0; i-- {
			if send(matches[i]) {
				return nil
			}
		}
		if ctx.Err() != nil || limitReached() {
			return nil
		}
		hi = lo - 1
		chunk = min(chunk*2, searchMaxChunk)
	}
	return nil
}

// consumeRange passes the records of [from, to] to process, until process asks to stop.
func consumeRange(
	ctx context.Context,
	client sarama.Consumer,
	topic string,
	partition int32,
	from, to int64,
	process func(*sarama.ConsumerMessage) (stop bool),
) error {
	consumer, err := client.ConsumePartition(topic, partition, from)
	if err != nil {
		return err
	}
	// the next range consumes the same partition, which requires this consumer to be closed
	defer func() { _ = consumer.Close() }()

	// to might never be delivered, e.g. for transaction control records
	idle := time.NewTimer(idleConsumptionTimeout)
	defer idle.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-consumer.Errors():
			return err
		case <-idle.C:
			if hwm := consumer.HighWaterMarkOffset(); hwm > 0 && hwm-1 <= to {
				return nil
			}
			idle.Reset(idleConsumptionTimeout)
		case msg := <-consumer.Messages():
			// records after the range are searched by the previous range already
			if msg.Offset > to {
				return nil
			}
			if process(msg) || msg.Offset == to {
				return nil
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleConsumptionTimeout)
		}
	}
}
