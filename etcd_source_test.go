package config

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/tests/v3/framework/integration"
)

func TestEtcdSource_Load(t *testing.T) {
	integration.BeforeTest(t)

	clus := integration.NewCluster(t, &integration.ClusterConfig{Size: 1})
	defer clus.Terminate(t)

	client := clus.RandClient()

	_, err := client.Put(context.Background(), "/app/config", "host: 127.0.0.1")
	require.NoError(t, err)

	t.Run("EtcdSource_Load_Success", func(t *testing.T) {
		source := NewEtcdSource(client, "/app/config",
			WithEtcdSourceReadTimeout(50*time.Second),
			WithEtcdSourceName("name"),
			WithEtcdSourceFormat("yaml"),
		)
		b, m, err := source.Load()
		assert.NoError(t, err)
		assert.NotNil(t, m)
		assert.NotNil(t, b)
		assert.Contains(t, string(b), "host: 127.0.0.1")
		assert.Equal(t, m.Format, "yaml")
	})

	_, err = client.Put(context.Background(), "/app/db.json", `{"host"": "127.0.0.1"}`)
	require.NoError(t, err)

	t.Run("EtcdSource_Load_AutoFormat", func(t *testing.T) {
		source := NewEtcdSource(client, "/app/db.json",
			WithEtcdSourceReadTimeout(50*time.Second),
			WithEtcdSourceName("name"),
		)
		b, m, err := source.Load()
		assert.NoError(t, err)
		assert.NotNil(t, m)
		assert.NotNil(t, b)
		assert.Contains(t, string(b), "127.0.0.1")
		assert.Equal(t, m.Format, "json")
	})
}
