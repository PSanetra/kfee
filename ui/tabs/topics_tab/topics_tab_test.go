package topics_tab

import (
	"kfee/kadmin"
	"kfee/serdes"
	"kfee/tests"
	"kfee/ui/components/statusbar"
	"kfee/ui/tabs"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestRecordsKeepArrivingWhileARecordIsViewed(t *testing.T) {
	ktx := tests.NewKontext()
	m, _ := New(ktx, kadmin.NewMockKadmin(), statusbar.New())
	topic := &kadmin.ListedTopic{Name: "orders", PartitionCount: 1}
	m.ToConsumePage(tabs.ConsumePageDetails{Topic: topic, ReadDetails: kadmin.NewDefaultReadDetails(topic)})
	now := time.Now()
	m.Update(kadmin.ConsumerRecordReceived{Records: []kadmin.ConsumerRecord{
		{Key: "first", Offset: 1, Timestamp: now, Payload: serdes.DesData{Value: "{}"}},
	}})
	m.View(ktx, tests.Renderer)

	record := kadmin.ConsumerRecord{Key: "first", Offset: 1, Timestamp: now}
	m.ToRecordDetailsPage(tabs.RecordDetailsPageDetails{Record: &record, TopicName: "orders"})
	m.Update(kadmin.ConsumerRecordReceived{Records: []kadmin.ConsumerRecord{
		{Key: "second", Offset: 2, Timestamp: now.Add(time.Second), Payload: serdes.DesData{Value: "{}"}},
	}})

	m.ToCachedConsumptionPage()
	render := ansi.Strip(m.View(ktx, tests.Renderer))
	assert.Contains(t, render, "first")
	assert.Contains(t, render, "second")
}
