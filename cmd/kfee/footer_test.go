package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"kfee/config"
	"kfee/kadmin"
	"kfee/serdes"
	"kfee/ui/tabs"
)

// A record taller than the window must not push the footer out of the window.
func TestFooterStaysInWindowForTallRecords(t *testing.T) {
	disableNerdFonts := true
	io := config.NewInMemoryConfigIO(&config.Config{Clusters: []config.Cluster{{Name: "prd", Color: "#808080", Active: true, BootstrapServers: []string{":19092"}}}})
	model := NewModel(&disableNerdFonts, kadmin.NewMockKadminInstantiator(), io)
	model.Update(config.LoadedMsg{Config: config.New(io)})
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	var headers []kadmin.Header
	for i := 0; i < 8; i++ {
		headers = append(headers, kadmin.Header{Key: fmt.Sprintf("h%d", i), Value: kadmin.NewHeaderValue("v")})
	}
	value := "{" + strings.Repeat(`"field":"value",`, 60) + `"x":1}`
	rec := kadmin.ConsumerRecord{Headers: headers, Payload: serdes.DesData{Value: value, RuleID: "cancelled", MessageType: "acme.Order"}}
	model.topicsTabCtrl.ToRecordDetailsPage(tabs.RecordDetailsPageDetails{Record: &rec, TopicName: "orders", Records: []kadmin.ConsumerRecord{rec}, Index: 0})
	model.tabCtrl = model.topicsTabCtrl
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	assert.LessOrEqual(t, len(lines), 40)
	assert.Contains(t, lines[len(lines)-1], "Protobuf Decoder")
}
