package transformation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormat(t *testing.T) {
	t.Run("pipes of the top level start their own line", func(t *testing.T) {
		assert.Equal(t, ".items\n| map({ id: .id })\n| sort_by(.id)", Format(".items | map({id: .id}) | sort_by(.id)"))
	})

	t.Run("a single term stays on one line", func(t *testing.T) {
		assert.Equal(t, `{ latest: .id }`, Format(`{latest:.id}`))
	})

	t.Run("a filter that does not parse is returned as it is", func(t *testing.T) {
		assert.Equal(t, `{latest: .id`, Format(`{latest: .id`))
	})
}
