package kadmin

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"kfee/serdes"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"charm.land/log/v2"

	tea "charm.land/bubbletea/v2"
	"github.com/IBM/sarama"
)

type FilterType string

// Matches reports whether a record satisfies every criterion set on the filter.
// The term matches the key, the value or a header value.
func (f *Filter) Matches(key, value string, headers []Header) bool {
	if f == nil {
		return true
	}
	if f.Term != "" && !containsTerm(strings.ToLower(f.Term), key, value, headers) {
		return false
	}
	return f.KeyFilter.matches(key, f.KeySearchTerm) &&
		f.ValueFilter.matches(value, f.ValueSearchTerm)
}

func containsTerm(term, key, value string, headers []Header) bool {
	if strings.Contains(strings.ToLower(key), term) || strings.Contains(strings.ToLower(value), term) {
		return true
	}
	for _, h := range headers {
		if strings.Contains(strings.ToLower(h.Value.String()), term) {
			return true
		}
	}
	return false
}

// IsActive reports whether the filter restricts the records, which makes reading
// search the whole selected range instead of only reading the limit.
func (f *Filter) IsActive() bool {
	if f == nil {
		return false
	}
	return f.Term != "" ||
		(f.KeyFilter.isValid() && f.KeySearchTerm != "") ||
		(f.ValueFilter.isValid() && f.ValueSearchTerm != "")
}

func (t FilterType) isValid() bool {
	return t == ContainsFilterType || t == StartsWithFilterType
}

func (t FilterType) matches(value, term string) bool {
	switch t {
	case ContainsFilterType:
		return strings.Contains(value, term)
	case StartsWithFilterType:
		return strings.HasPrefix(value, term)
	default:
		return true
	}
}

const (
	ContainsFilterType   FilterType = "contains"
	StartsWithFilterType FilterType = "starts with"
	NoFilterType         FilterType = "none"
)

type StartPoint int64

// idleConsumptionTimeout is how long a partition may go without delivering a message
// before checking whether all available records have been consumed.
var idleConsumptionTimeout = 3 * time.Second

const (
	Beginning StartPoint = iota
	MostRecent
	Today
	Yesterday
	Last7Days
	Live
)

func (p *StartPoint) time() int64 {
	switch *p {
	case Beginning, MostRecent, Live:
		return sarama.OffsetOldest
	case Today:
		t := time.Now()
		startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		return startOfDay.UnixMilli()
	case Yesterday:
		t := time.Now().AddDate(0, 0, -1)
		startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		return startOfDay.UnixMilli()
	case Last7Days:
		t := time.Now().AddDate(0, 0, -7)
		startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		return startOfDay.UnixMilli()
	}

	return int64(*p)
}

type RecordReader interface {
	ReadRecords(ctx context.Context, rd ReadDetails) tea.Msg
}

type ReadingStartedMsg struct {
	ConsumerRecord chan ConsumerRecord
	EmptyTopic     chan bool
	// NoRecordsFound indicates that there are no records in any of the selected partitions
	// for the given filter criteria.
	NoRecordsFound chan bool
	Err            chan error
	CancelFunc     context.CancelFunc
	// Progress counts the searched records when a filter is set, nil otherwise
	Progress *SearchProgress
}

// SearchProgress counts the records searched for matches of a filter.
type SearchProgress struct {
	scanned atomic.Int64
	total   atomic.Int64
}

// Scanned is the number of records searched so far.
func (p *SearchProgress) Scanned() int64 { return p.scanned.Load() }

// Total is the number of records to search at most.
func (p *SearchProgress) Total() int64 { return p.total.Load() }

func (m *ReadingStartedMsg) AwaitRecord() tea.Msg {
	select {
	case record, ok := <-m.ConsumerRecord:
		if !ok {
			return ConsumptionEndedMsg{source: m.ConsumerRecord}
		}

		return ConsumerRecordReceived{
			Records:        []ConsumerRecord{record},
			consumerRecord: m.ConsumerRecord,
			emptyTopic:     m.EmptyTopic,
			noRecordsFound: m.NoRecordsFound,
			err:            m.Err,
			cancelFunc:     m.CancelFunc,
		}
	case empty := <-m.EmptyTopic:
		if empty {
			return EmptyTopicMsg{}
		}
		return nil
	case noRecords := <-m.NoRecordsFound:
		if noRecords {
			return NoRecordsFound{
				consumerRecord: m.ConsumerRecord,
				emptyTopic:     m.EmptyTopic,
				noRecordsFound: m.NoRecordsFound,
				err:            m.Err,
				cancelFunc:     m.CancelFunc,
			}
		}
		return nil
	case err := <-m.Err:
		return err
	}
}

// shutdown signals the end of the consumption by closing the record channel.
// The other channels are deliberately left open, as a closed channel is always
// ready in a select and would race with the closed record channel.
func (m *ReadingStartedMsg) shutdown() {
	m.CancelFunc()
	close(m.ConsumerRecord)
}

type ConsumerRecordReceived struct {
	Records        []ConsumerRecord
	consumerRecord chan ConsumerRecord
	emptyTopic     chan bool
	noRecordsFound chan bool
	err            chan error
	cancelFunc     context.CancelFunc
}

func (m *ConsumerRecordReceived) AwaitNextRecord() tea.Msg {
	log.Debug("awaiting next record")
	select {
	case record, ok := <-m.consumerRecord:
		if !ok {
			return ConsumptionEndedMsg{source: m.consumerRecord}
		}

		records := []ConsumerRecord{record}
		timeout := time.After(50 * time.Millisecond)
		for {
			select {
			case r, ok := <-m.consumerRecord:
				if !ok {
					// consumption ended, the next await reports it
					return ConsumerRecordReceived{
						Records:        records,
						consumerRecord: m.consumerRecord,
						emptyTopic:     m.emptyTopic,
						noRecordsFound: m.noRecordsFound,
						err:            m.err,
						cancelFunc:     m.cancelFunc,
					}
				}
				records = append(records, r)
			case <-timeout:
				return ConsumerRecordReceived{
					Records:        records,
					consumerRecord: m.consumerRecord,
					emptyTopic:     m.emptyTopic,
					noRecordsFound: m.noRecordsFound,
					err:            m.err,
					cancelFunc:     m.cancelFunc,
				}
			}
		}

	case emptyTopic := <-m.emptyTopic:
		if emptyTopic {
			return EmptyTopicMsg{}
		}
		return nil
	case err := <-m.err:
		return err
	}
}

type EmptyTopicMsg struct {
}

type NoRecordsFound struct {
	consumerRecord chan ConsumerRecord
	emptyTopic     chan bool
	noRecordsFound chan bool
	err            chan error
	cancelFunc     context.CancelFunc
}

type ConsumptionEndedMsg struct {
	source chan ConsumerRecord
}

// Source identifies the consumption a message belongs to. It is nil for
// messages not tied to a specific consumption.
func (m ConsumptionEndedMsg) Source() chan ConsumerRecord { return m.source }

func (m ConsumerRecordReceived) Source() chan ConsumerRecord { return m.consumerRecord }

func (m *ReadingStartedMsg) Source() chan ConsumerRecord { return m.ConsumerRecord }

type Filter struct {
	// Term matches records whose key or value contains it, ignoring case.
	Term            string
	KeyFilter       FilterType
	KeySearchTerm   string
	ValueFilter     FilterType
	ValueSearchTerm string
}

type ReadDetails struct {
	TopicName       string
	PartitionToRead []int
	StartPoint      StartPoint
	Limit           int
	Filter          *Filter
}

// limit is the maximum number of records to read, DefaultLimit when no limit is set.
func (rd ReadDetails) limit() int64 {
	if rd.Limit <= 0 {
		return DefaultLimit
	}
	return int64(rd.Limit)
}

// DefaultLimit is the number of records read by default.
const DefaultLimit = 500

type HeaderValue struct {
	data []byte
}

func NewDefaultReadDetails(topic *ListedTopic) ReadDetails {
	return ReadDetails{
		TopicName:       topic.Name,
		PartitionToRead: topic.Partitions(),
		StartPoint:      MostRecent,
		Limit:           DefaultLimit,
		Filter:          &Filter{},
	}
}

func NewHeaderValue(data string) HeaderValue {
	return HeaderValue{[]byte(data)}
}

// Bytes returns the raw header value.
func (v HeaderValue) Bytes() []byte {
	return v.data
}

func (v HeaderValue) String() string {
	if utf8.Valid(v.data) {
		return string(v.data)
	}

	if len(v.data) >= 4 {
		var int32Val int32
		err := binary.Read(bytes.NewReader(v.data), binary.BigEndian, &int32Val)
		if err == nil {
			return string(int32Val)
		}
	}
	if len(v.data) >= 8 {
		var int64Val int64
		err := binary.Read(bytes.NewReader(v.data), binary.BigEndian, &int64Val)
		if err == nil {
			return strconv.FormatInt(int64Val, 10)
		}
	}

	if len(v.data) >= 4 {
		var float32Val float32
		err := binary.Read(bytes.NewReader(v.data), binary.BigEndian, &float32Val)
		if err == nil {
			return strconv.FormatFloat(float64(float32Val), 'f', -1, 32)
		}
	}
	if len(v.data) >= 8 {
		var float64Val float64
		err := binary.Read(bytes.NewReader(v.data), binary.BigEndian, &float64Val)
		if err == nil {
			return strconv.FormatFloat(float64Val, 'f', -1, 64)
		}
	}

	return string(v.data)
}

type Header struct {
	Key   string
	Value HeaderValue
}

type ConsumerRecord struct {
	Key     string
	Payload serdes.DesData
	// RawValue is the record value as consumed, used to decode the record again
	RawValue  []byte
	Err       error
	Partition int64
	Offset    int64
	Headers   []Header
	Timestamp time.Time
	// protobuf reports whether the record was handled by the protobuf decoders
	protobuf bool
}

// RedecodeProtobuf decodes the record again with the given protobuf decoders, as needed after
// their configuration changed. Records not handled by the decoders keep their payload, unless
// they were handled by them before, then they fall back to plain text.
func (record ConsumerRecord) RedecodeProtobuf(decoders *serdes.ProtobufDecoders, topic string) ConsumerRecord {
	data, handled, err := decoders.Decode(topic, serdesHeaders(record.Headers), record.RawValue)
	switch {
	case handled:
		record.Payload, record.Err, record.protobuf = data, err, true
	case record.protobuf:
		record.Payload, record.Err, record.protobuf = serdes.DesData{Value: string(record.RawValue)}, nil, false
	}
	return record
}

func serdesHeaders(headers []Header) []serdes.Header {
	result := make([]serdes.Header, 0, len(headers))
	for _, h := range headers {
		result = append(result, serdes.Header{Key: h.Key, Value: h.Value.data})
	}
	return result
}

func (record *ConsumerRecord) PayloadType() string {
	if record.Payload.MessageType == serdes.RawMessageType {
		return "Protobuf (raw)"
	}
	if record.Payload.MessageType != "" {
		return "Protobuf"
	}

	// schema is not empty, so it's Avro
	if record.Payload.Schema != "" {
		return "Avro"
	}

	// value is empty, so it's plain text'
	value := strings.TrimSpace(record.Payload.Value)
	if value == "" {
		return "Plain Text"
	}

	// value is a valid json, so it's a json'
	if json.Valid([]byte(value)) {
		return "Plain Json"
	}

	// value is a valid xml, so it's a xml'
	if isValidXML([]byte(value)) {
		return "Plain XML"
	}

	// default value is plain text
	return "Plain Text"
}

func isValidXML(data []byte) bool {
	err := xml.Unmarshal(data, new(interface{}))
	return err == nil
}

type offsets struct {
	start int64
	// end is the offset after the newest record, i.e. the high water mark
	end int64
}

func (o *offsets) newest() int64 {
	return o.end - 1
}

func (ka *SaramaKafkaAdmin) ReadRecords(ctx context.Context, rd ReadDetails) tea.Msg {
	ctx, cancelFunc := context.WithCancel(ctx)
	startedMsg := &ReadingStartedMsg{
		ConsumerRecord: make(chan ConsumerRecord, len(rd.PartitionToRead)),
		Err:            make(chan error, 1),
		EmptyTopic:     make(chan bool, 1),
		NoRecordsFound: make(chan bool, 1),
		CancelFunc:     cancelFunc,
	}
	if rd.isSearch() {
		startedMsg.Progress = &SearchProgress{}
	}

	go ka.doReadRecords(ctx, rd, startedMsg, cancelFunc)
	return startedMsg
}

// isSearch reports whether the records are searched: with a filter, the whole selected range is
// read until the limit of matching records is reached, instead of only reading the limit.
func (rd ReadDetails) isSearch() bool {
	return rd.StartPoint != Live && rd.Filter.IsActive()
}

const (
	// searchInitialChunk and searchMaxChunk size the ranges searched backwards from the most recent record.
	searchInitialChunk int64 = 500
	searchMaxChunk     int64 = 20000
)

func (ka *SaramaKafkaAdmin) doReadRecords(
	ctx context.Context,
	rd ReadDetails,
	startedMsg *ReadingStartedMsg,
	cancelFunc context.CancelFunc,
) {
	client, err := sarama.NewConsumerFromClient(ka.client)
	if err != nil {
		startedMsg.Err <- err
		cancelFunc()
		return
	}

	var (
		msgCount atomic.Int64
		wg       sync.WaitGroup
		offsets  map[int]offsets
	)

	offsets, err = ka.fetchOffsets(rd.PartitionToRead, rd.TopicName, rd.StartPoint)
	if err != nil {
		startedMsg.Err <- err
		cancelFunc()
		return
	}

	if noRecordsFound(offsets) {
		cancelFunc()
		startedMsg.NoRecordsFound <- true
		return
	}

	if progress := startedMsg.Progress; progress != nil {
		for _, p := range rd.PartitionToRead {
			if off := offsets[p]; off.start >= 0 && off.end > off.start {
				progress.total.Add(off.end - off.start)
			}
		}
	}

	// match decodes a consumed record and reports whether it matches the filter
	match := func(msg *sarama.ConsumerMessage) (ConsumerRecord, bool) {
		if progress := startedMsg.Progress; progress != nil {
			progress.scanned.Add(1)
		}

		var headers []Header
		for _, h := range msg.Headers {
			headers = append(headers, Header{
				string(h.Key),
				HeaderValue{h.Value},
			})
		}

		key := string(msg.Key)
		desData, protobuf, err := ka.deserialize(msg.Topic, headers, msg.Value)

		// non-matching records are dropped right away
		if rd.Filter != nil && err == nil && !ka.matchesRecord(rd.Filter, msg.Topic, key, desData, headers) {
			return ConsumerRecord{}, false
		}

		return ConsumerRecord{
			Key:       key,
			Payload:   desData,
			RawValue:  msg.Value,
			Err:       err,
			Partition: int64(msg.Partition),
			Offset:    msg.Offset,
			Headers:   headers,
			Timestamp: msg.Timestamp,
			protobuf:  protobuf,
		}, true
	}

	// the oldest record of each partition, which orders the partitions when reading further back
	oldest := newOldestTimes()
	countingMatch := func(msg *sarama.ConsumerMessage) (ConsumerRecord, bool) {
		record, ok := match(msg)
		if ok {
			oldest.observe(int(msg.Partition), msg.Timestamp)
		}
		return record, ok
	}

	// send passes a record to the UI, it reports whether reading has to stop
	send := func(consumerRecord ConsumerRecord) (stop bool) {
		// partitions are read concurrently, only the record reaching the limit is still sent
		if count := msgCount.Add(1); count >= rd.limit() {
			if count == rd.limit() {
				select {
				case startedMsg.ConsumerRecord <- consumerRecord:
				case <-ctx.Done():
				}
			}
			return true
		}

		select {
		case startedMsg.ConsumerRecord <- consumerRecord:
			return false
		case <-ctx.Done():
			return true
		}
	}

	process := func(msg *sarama.ConsumerMessage) (stop bool) {
		if record, ok := countingMatch(msg); ok {
			return send(record)
		}
		return false
	}

	// readWindow consumes the offsets of one window of a partition, until its end or the limit
	readWindow := func(partition int, window readingOffsets) {
		log.Debug("Reading offsets determined",
			"topic", rd.TopicName,
			"partition", partition,
			"start", window.start,
			"end", window.end,
		)

		consumer, err := client.ConsumePartition(
			rd.TopicName,
			int32(partition),
			window.start,
		)
		if err != nil {
			sendErr(ctx, startedMsg.Err, err)
			cancelFunc()
			return
		}
		// Closing waits for the in-flight fetch, which the broker holds for
		// Consumer.MaxWaitTime when there is no new data, so close in the background.
		defer func() {
			consumer.AsyncClose()
			go func() {
				for range consumer.Messages() {
				}
			}()
			go func() {
				for range consumer.Errors() {
				}
			}()
		}()

		msgChan := consumer.Messages()
		live := rd.StartPoint == Live

		// The window end may be a record that is never delivered (e.g. transaction control
		// markers). So when no message arrives for a while, we verify that all records up to the
		// high water mark have been fetched.
		idle := time.NewTimer(idleConsumptionTimeout)
		defer idle.Stop()

		for {
			select {
			case err := <-consumer.Errors():
				sendErr(ctx, startedMsg.Err, err)
				return
			case <-ctx.Done():
				return
			case <-idle.C:
				hwm := consumer.HighWaterMarkOffset()
				if idleReachedEnd(live, hwm, len(msgChan)) {
					log.Debug("No more records available",
						"topic", rd.TopicName,
						"partition", partition,
						"end", window.end,
						"highWaterMark", hwm,
					)
					return
				}
				idle.Reset(idleConsumptionTimeout)
			case msg := <-msgChan:
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(idleConsumptionTimeout)

				reachedEnd := !live && msg.Offset >= window.end
				if process(msg) {
					return
				}
				if reachedEnd {
					return
				}
			}
		}
	}

	emptyTopic := true

	log.Debug("Starting to read records",
		"partition", rd.PartitionToRead,
		"offsets", offsets)

	// windows are the offsets covered so far by each partition
	windows := make(map[int]readingOffsets)

	for _, p := range rd.PartitionToRead {
		// if there is no data in the partition, we don't need to read it unless live consumption is requested
		partition := p
		if offsets[partition].end != offsets[partition].start || rd.StartPoint == Live {

			emptyTopic = false

			if rd.isSearch() && rd.StartPoint == MostRecent {
				wg.Go(func() {
					err := ka.searchBackwards(ctx, client, rd.TopicName, int32(partition), offsets[partition], countingMatch, send, func() bool {
						return msgCount.Load() >= rd.limit()
					})
					if err != nil {
						sendErr(ctx, startedMsg.Err, err)
					}
				})
				continue
			}

			window := ka.determineReadingOffsets(rd, offsets[partition])
			windows[partition] = window
			wg.Go(func() { readWindow(partition, window) })
		}
	}

	if emptyTopic {
		cancelFunc()
		startedMsg.EmptyTopic <- true
		return
	}

	// extendWindows reads further back (forward for Beginning) until the limit is reached or no
	// offsets are left. Each round gives the missing records evenly to the partitions that have
	// more offsets, the partitions with the youngest oldest record first. A round can not deliver
	// more records than offsets it reads, so the limit is never exceeded.
	extendWindows := func() {
		backward := rd.StartPoint != Beginning
		for ctx.Err() == nil {
			deficit := rd.limit() - msgCount.Load()
			if deficit <= 0 {
				return
			}
			candidates := extensionCandidates(rd.PartitionToRead, windows, offsets, backward)
			if len(candidates) == 0 {
				return
			}
			candidates = rankForExtension(candidates, oldest.snapshot())
			base, remainder := deficit/int64(len(candidates)), deficit%int64(len(candidates))

			var round sync.WaitGroup
			for i, p := range candidates {
				size := base
				if int64(i) < remainder {
					size++
				}
				ext, covered, ok := nextWindow(windows[p], offsets[p], backward, size)
				if !ok {
					continue
				}
				windows[p] = covered
				partition := p
				round.Go(func() { readWindow(partition, ext) })
			}
			round.Wait()
		}
	}

	go func() {
		wg.Wait()
		if !rd.isSearch() && rd.StartPoint != Live {
			extendWindows()
		}
		time.Sleep(50 * time.Millisecond)
		startedMsg.shutdown()
	}()
}

// sendErr reports err without blocking when the error channel is full and consumption got cancelled.
func sendErr(ctx context.Context, errChan chan error, err error) {
	select {
	case errChan <- err:
	case <-ctx.Done():
	}
}

func noRecordsFound(offsets map[int]offsets) bool {
	for _, off := range offsets {
		// -1 indicates that no records exist for the requested offsets
		if off.start != -1 {
			return false
		}
	}
	return true
}

// deserialize decodes the record value, protobuf reports whether it was handled by the protobuf decoders.
func (ka *SaramaKafkaAdmin) deserialize(
	topic string,
	headers []Header,
	value []byte,
) (data serdes.DesData, protobuf bool, err error) {
	if data, handled, err := ka.protoDecoders.Decode(topic, serdesHeaders(headers), value); handled {
		return data, true, err
	}
	deserializer := serdes.NewAvroDeserializer(ka.sra)
	data, err = deserializer.Deserialize(value)
	return data, false, err
}

type readingOffsets struct {
	start int64
	end   int64
}

// idleReachedEnd reports whether a partition may stop after the idle timeout. A response with a
// high water mark arrived and no message is waiting, so every record up to the high water mark
// has been delivered. It must not stop while fetched messages are still waiting to be read.
func idleReachedEnd(live bool, hwm int64, pending int) bool {
	return !live && hwm > 0 && pending == 0
}

func (ka *SaramaKafkaAdmin) determineReadingOffsets(
	rd ReadDetails,
	offsets offsets,
) readingOffsets {

	if rd.StartPoint == Live {
		return readingOffsets{
			start: offsets.end,
			end:   -1,
		}
	}

	var startOffset int64
	var endOffset int64
	// searching reads the whole range, the limit applies to the matching records
	if rd.isSearch() {
		return readingOffsets{
			start: offsets.start,
			end:   offsets.newest(),
		}
	}
	// at least one record per partition, the total is capped by the limit while reading
	numberOfRecordsPerPart := max(rd.limit()/int64(len(rd.PartitionToRead)), 1)
	if rd.StartPoint == Beginning {
		startOffset, endOffset = ka.determineOffsetsFromBeginning(
			offsets,
			numberOfRecordsPerPart,
		)
	} else {
		startOffset, endOffset = ka.determineMostRecentOffsets(
			startOffset,
			offsets,
			numberOfRecordsPerPart,
			endOffset,
		)
	}
	return readingOffsets{
		start: startOffset,
		end:   endOffset,
	}
}

func (ka *SaramaKafkaAdmin) determineMostRecentOffsets(
	startOffset int64,
	offsets offsets,
	numberOfRecordsPerPart int64,
	endOffset int64,
) (int64, int64) {
	// both offsets are inclusive
	startOffset = offsets.newest() - numberOfRecordsPerPart + 1
	endOffset = offsets.newest()
	if startOffset < 0 || startOffset < offsets.start {
		startOffset = offsets.start
	}
	return startOffset, endOffset
}

func (ka *SaramaKafkaAdmin) determineOffsetsFromBeginning(
	offsets offsets,
	numberOfRecordsPerPart int64,
) (int64, int64) {
	// both offsets are inclusive
	startOffset := offsets.start
	endOffset := min(startOffset+numberOfRecordsPerPart-1, offsets.newest())
	return startOffset, endOffset
}

func (ka *SaramaKafkaAdmin) fetchOffsets(
	partitions []int,
	topicName string,
	startPoint StartPoint,
) (map[int]offsets, error) {
	offsetsByPartition := make(map[int]offsets)
	var wg sync.WaitGroup
	var mu sync.Mutex
	errorsChan := make(chan error, len(partitions))

	for _, p := range partitions {

		partition := p

		log.Debug("Fetching offsets", "topic", topicName, "partition", partition)

		wg.Go(func() {
			startOffset, err := ka.client.GetOffset(
				topicName,
				int32(partition),
				startPoint.time(),
			)
			if err != nil {
				errorsChan <- err
				return
			}
			log.Debug(
				"Fetched start offset",
				"topic", topicName,
				"partition", partition,
				"startOffset", startOffset,
			)

			endOffset, err := ka.client.GetOffset(
				topicName,
				int32(partition),
				sarama.OffsetNewest,
			)
			if err != nil {
				errorsChan <- err
				return
			}
			log.Debug("Fetched end offset",
				"topic", topicName,
				"partition", partition,
				"endOffset", endOffset)

			off := offsets{start: startOffset, end: endOffset}

			mu.Lock()
			offsetsByPartition[partition] = off
			mu.Unlock()
		})
	}

	wg.Wait()

	select {
	case err := <-errorsChan:
		return nil, err
	default:
		return offsetsByPartition, nil
	}
}
