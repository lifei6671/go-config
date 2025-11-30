package decoder

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTOMLDecoder_Decode(t *testing.T) {
	t.Run("TOMLDecoder_Decode", func(t *testing.T) {
		b, err := os.ReadFile("../testdata/conf/abc.toml")
		assert.NoError(t, err)
		d := TOMLDecoder{}
		ret, err := d.Decode(b)
		assert.NoError(t, err)
		assert.Contains(t, ret, "A")
	})
	t.Run("TOMLDecoder_Decode_Empty", func(t *testing.T) {
		d := TOMLDecoder{}
		_, err := d.Decode([]byte{})
		assert.Nil(t, err)
	})

	t.Run("TOMLDecoder_Decode_Error", func(t *testing.T) {
		b, err := os.ReadFile("../testdata/conf/abc.json")
		assert.NoError(t, err)
		d := TOMLDecoder{}
		ret, err := d.Decode(b)
		assert.NotNil(t, err)
		assert.Nil(t, ret)
	})
}
