package kadmin

import (
	"errors"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/IBM/sarama"
	"github.com/burdiyan/kafkautil"
)

type Publisher interface {
	PublishRecord(p *ProducerRecord) PublicationStartedMsg
	// PublishBatch publishes the records without waiting for each acknowledgement, see SaramaKafkaAdmin.
	PublishBatch(records []*ProducerRecord) error
}

type ProducerRecord struct {
	Key       string
	Value     []byte
	Topic     string
	Partition *int
	Headers   map[string]string
	Timestamp time.Time
}

type PublicationStartedMsg struct {
	Err       chan error
	Published chan bool
}

type PublicationFailed struct {
	Err error
}

type PublicationSucceeded struct {
}

func (p *PublicationStartedMsg) AwaitCompletion() tea.Msg {
	select {
	case err := <-p.Err:
		return PublicationFailed{err}
	case <-p.Published:
		return PublicationSucceeded{}
	}
}

func (ka *SaramaKafkaAdmin) PublishRecord(p *ProducerRecord) PublicationStartedMsg {
	errChan := make(chan error)
	published := make(chan bool)

	go ka.doPublishRecord(p, errChan, published)

	return PublicationStartedMsg{
		Err:       errChan,
		Published: published,
	}
}

// PublishBatch queues all records on an async producer, so the sends overlap instead of waiting for
// each acknowledgement. It returns when every record is acknowledged, or with the errors of the records
// that failed. The records must not set a partition, they are partitioned like PublishRecord does.
func (ka *SaramaKafkaAdmin) PublishBatch(records []*ProducerRecord) error {
	MaybeIntroduceLatency()
	ka.config.Producer.Partitioner = kafkautil.NewJVMCompatiblePartitioner

	producer, err := sarama.NewAsyncProducerFromClient(ka.client)
	if err != nil {
		return err
	}

	// the sync producer of the admin requires the successes, the async producer returns them too
	var wg sync.WaitGroup
	var errs []error
	wg.Go(func() {
		for range producer.Successes() {
		}
	})
	wg.Go(func() {
		for e := range producer.Errors() {
			errs = append(errs, e)
		}
	})

	for _, p := range records {
		producer.Input() <- &sarama.ProducerMessage{
			Topic:     p.Topic,
			Key:       sarama.StringEncoder(p.Key),
			Value:     sarama.ByteEncoder(p.Value),
			Headers:   recordHeaders(p.Headers),
			Timestamp: p.Timestamp,
		}
	}
	producer.AsyncClose()
	wg.Wait()

	return errors.Join(errs...)
}

func recordHeaders(headers map[string]string) []sarama.RecordHeader {
	var out []sarama.RecordHeader
	for key, value := range headers {
		out = append(out, sarama.RecordHeader{
			Key:   []byte(key),
			Value: []byte(value),
		})
	}
	return out
}

func (ka *SaramaKafkaAdmin) doPublishRecord(
	p *ProducerRecord,
	errChan chan error,
	published chan bool,
) {
	MaybeIntroduceLatency()
	var partition int32
	if p.Partition == nil {
		ka.config.Producer.Partitioner = kafkautil.NewJVMCompatiblePartitioner
	} else {
		partition = int32(*p.Partition)
		ka.config.Producer.Partitioner = sarama.NewManualPartitioner
	}

	var headers []sarama.RecordHeader
	for key, value := range p.Headers {
		headers = append(headers, sarama.RecordHeader{
			Key:   []byte(key),
			Value: []byte(value),
		})
	}

	_, _, err := ka.producer.SendMessage(&sarama.ProducerMessage{
		Topic:     p.Topic,
		Key:       sarama.StringEncoder(p.Key),
		Value:     sarama.ByteEncoder(p.Value),
		Partition: partition,
		Headers:   headers,
		Timestamp: p.Timestamp,
	})
	if err != nil {
		errChan <- err
		return
	}
	published <- true
}
