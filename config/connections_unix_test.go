//go:build unix

package config

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnectionsFilePermissionsIgnoreUmask(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	io, configFile, connectionsFile := testIO(t)

	_, err := io.read()
	require.NoError(t, err)
	require.NoError(t, io.write(&Config{Clusters: []Cluster{prdCluster()}}))

	assertPerm(t, connectionsFile, 0o600)
	assertPerm(t, filepath.Dir(configFile), 0o700)
}
