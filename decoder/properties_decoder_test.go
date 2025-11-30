package decoder

import (
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPropertiesDecoder_Decode(t *testing.T) {
	t.Run("PropertiesDecoder_Decode_success", func(t *testing.T) {
		b, err := os.ReadFile("../testdata/conf/abc.properties")
		assert.NoError(t, err)

		d := PropertiesDecoder{}
		ret, err := d.Decode(b)

		assert.NoError(t, err)
		assert.Contains(t, ret, "A")
		val, ok := ret["database.user"]
		assert.True(t, ok)
		log.Println(val)
		assert.Equal(t, val, "root")
	})
}
