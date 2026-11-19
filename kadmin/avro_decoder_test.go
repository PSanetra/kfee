package kadmin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPickSubject(t *testing.T) {
	assert.Equal(t, "orders-value", pickSubject([]string{"other", "orders-value", "orders-key"}, "orders"))
	assert.Equal(t, "orders-key", pickSubject([]string{"orders-key", "other"}, "orders"))
	assert.Equal(t, "other", pickSubject([]string{"other"}, "orders"))
}
