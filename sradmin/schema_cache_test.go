package sradmin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"kfee/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaIDsAreQueriedOncePerSession(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var mu sync.Mutex
	requests := map[string]int{}
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/schemas/ids/1":
			fmt.Fprint(w, `{"schema":"{\"type\":\"string\"}","id":1}`)
		case "/schemas/ids/1/versions":
			fmt.Fprint(w, `[{"subject":"orders-value","version":1},{"subject":"other-value","version":3}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer registry.Close()
	cfg := &config.SchemaRegistryConfig{Url: registry.URL}

	client := New(cfg)
	for range 3 {
		msg := client.GetSchemaById(1)
		if pending, ok := msg.(GettingSchemaByIdMsg); ok {
			msg = pending.AwaitCompletion()
		}
		received, ok := msg.(SchemaByIdReceived)
		require.True(t, ok, "schema is received, got %T", msg)
		assert.Equal(t, `{"type":"string"}`, received.Schema.Value)

		subjects, err := client.SubjectsOfSchema(1)
		require.NoError(t, err)
		assert.Equal(t, []string{"orders-value", "other-value"}, subjects)
	}
	assert.Equal(t, 1, requests["/schemas/ids/1"])
	assert.Equal(t, 1, requests["/schemas/ids/1/versions"])

	// a new session reads the cache directory instead of the registry
	session := New(cfg)
	msg := session.GetSchemaById(1)
	assert.IsType(t, SchemaByIdReceived{}, msg)
	subjects, err := session.SubjectsOfSchema(1)
	require.NoError(t, err)
	assert.Equal(t, []string{"orders-value", "other-value"}, subjects)
	assert.Equal(t, 1, requests["/schemas/ids/1"])
	assert.Equal(t, 1, requests["/schemas/ids/1/versions"])
}
