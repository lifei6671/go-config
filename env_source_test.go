package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvSource_Load(t *testing.T) {
	t.Parallel()
	t.Run("EnvSource_Load_Success", func(t *testing.T) {
		source := NewEnvSource(
			WithEnvSourceEnviron(func() []string {
				return []string{
					"MYAPP=127.0.0.1",
					"MYAPP_DB__HOST=127.0.0.1",
					"MYAPP_DB__PORT=3306",
					"HOST",
					"HOST=127.0.0.1",
				}
			}),
			WithEnvSourceName("MYAPP_DB__HOST"),
			WithEnvSourceSeparator("__"),
			WithEnvSourceStripPrefix(true),
			WithEnvSourcePrefix("MYAPP"),
		)
		b, m, err := source.Load()
		assert.NoError(t, err)
		assert.NotNil(t, m)
		assert.NotNil(t, b)
	})
}
