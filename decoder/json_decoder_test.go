package decoder

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONDecoder_Decode(t *testing.T) {
	t.Run("JSONDecoder_Decode", func(t *testing.T) {
		b, err := os.ReadFile("../testdata/conf/abc.json")
		assert.NoError(t, err)
		d := JSONDecoder{}
		ret, err := d.Decode(b)
		assert.NoError(t, err)
		assert.Contains(t, ret, "A")
	})
	t.Run("JSONDecoder_Decode_Empty", func(t *testing.T) {
		d := JSONDecoder{}
		_, err := d.Decode([]byte{})
		assert.Nil(t, err)
	})

	t.Run("JSONDecoder_Decode_Error", func(t *testing.T) {
		b, err := os.ReadFile("../testdata/conf/abc.toml")
		assert.NoError(t, err)
		d := JSONDecoder{}
		ret, err := d.Decode(b)
		assert.NotNil(t, err)
		assert.Nil(t, ret)
	})
}
