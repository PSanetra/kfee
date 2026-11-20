package serdes

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeMessageType(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc is not installed")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "orders.proto"), []byte(`syntax = "proto3";
package acme;
enum Status { STATUS_UNKNOWN = 0; STATUS_CREATED = 1; }
message Item { int64 sku = 1; }
message Order {
  string id = 1;
  Status status = 2;
  repeated Item items = 3;
  map<string, int32> codes = 4;
}
`), 0o644))
	schema, err := CompileProtoDir("protoc", dir)
	require.NoError(t, err)

	described, err := schema.Describe("acme.Order")

	require.NoError(t, err)
	assert.Contains(t, described, "message acme.Order {\n  string id = 1;\n  acme.Status status = 2;\n  repeated acme.Item items = 3;\n  map<string, int32> codes = 4;\n}")
	assert.Contains(t, described, "enum acme.Status {\n  STATUS_UNKNOWN = 0;\n  STATUS_CREATED = 1;\n}")
	assert.Contains(t, described, "message acme.Item {\n  int64 sku = 1;\n}")

	_, err = schema.Describe("acme.Missing")
	assert.Error(t, err)
}
