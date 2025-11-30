package decoder

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestYAMLDecoder_Decode(t *testing.T) {
	t.Run("YAMLDecoder_Decode_Success", func(t *testing.T) {
		b, err := os.ReadFile("../testdata/conf/abc.yaml")
		assert.NoError(t, err)
		ret, err := YAMLDecoder{}.Decode(b)
		assert.NoError(t, err)
		assert.NotNil(t, ret)
	})

	t.Run("YAMLDecoder_Decode_Failure", func(t *testing.T) {
		b, err := os.ReadFile("../testdata/conf/abc.properties")
		assert.NoError(t, err)
		ret, err := YAMLDecoder{}.Decode(b)
		assert.NotNil(t, err)
		assert.Nil(t, ret)
	})
}
