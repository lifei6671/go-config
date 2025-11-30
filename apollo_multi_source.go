package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ApolloNamespaceConfig struct {
	Namespace string // e.g. "application", "db.yaml"
	Format    string // json/yaml/toml/properties
}

type ApolloMultiSource struct {
	BaseURL string
	AppID   string
	Cluster string

	Namespaces []ApolloNamespaceConfig

	client *http.Client
	name   string

	// cache: 保存每个 namespace 的最新配置，用于 fallback
	cache *ApolloCache
}

// ApolloCache 用来缓存每个 namespace 的最新配置内容
// 并用于 fallback（Apollo 出现失败时使用最后成功版本）
type ApolloCache struct {
	mu   sync.RWMutex
	data map[string][]byte // namespace -> config bytes(JSON)
}

func NewApolloCache() *ApolloCache {
	return &ApolloCache{
		data: make(map[string][]byte),
	}
}

func (c *ApolloCache) Set(ns string, content []byte) {
	c.mu.Lock()
	c.data[ns] = content
	c.mu.Unlock()
}

func (c *ApolloCache) Get(ns string) ([]byte, bool) {
	c.mu.RLock()
	b, ok := c.data[ns]
	c.mu.RUnlock()
	return b, ok
}

var _ Source = (*ApolloMultiSource)(nil)

func NewApolloMultiSource(baseURL, appID, cluster string, namespaces []ApolloNamespaceConfig, opts ...ApolloSourceOption) *ApolloMultiSource {

	src := &ApolloMultiSource{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		AppID:      appID,
		Cluster:    cluster,
		Namespaces: namespaces,
		client:     &http.Client{Timeout: 5 * time.Second},
		cache:      NewApolloCache(),
		name:       "",
	}

	for _, opt := range opts {
		opt(nil) // ignore options not applicable
	}

	if src.name == "" {
		src.name = fmt.Sprintf("apollo-multi[%s]", src.AppID)
	}

	return src
}

// 拉取单个 namespace 内容
func (a *ApolloMultiSource) fetchNamespace(namespace string) ([]byte, error) {
	url := fmt.Sprintf(
		"%s/configs/%s/%s/%s",
		a.BaseURL,
		a.AppID,
		a.Cluster,
		namespace,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ApolloMultiSource: new request failed: %w", err)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Apollo JSON 格式
	var obj struct {
		Configurations map[string]string `json:"configurations"`
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}

	// kv → map[string]any
	m := make(map[string]any)
	for k, v := range obj.Configurations {
		m[k] = v
	}

	// 序列化为 JSON 提供给 Config decoder
	return json.Marshal(m)
}

// Load 实现 Source：
// 加载所有 namespace 的内容，并合并成一个 JSON map：
//
//	{
//	   "<namespace1>": { ... },
//	   "<namespace2>": { ... }
//	}
func (a *ApolloMultiSource) Load() ([]byte, Metadata, error) {
	final := make(map[string]any)

	for _, ns := range a.Namespaces {
		b, err := a.fetchNamespace(ns.Namespace)
		if err != nil {
			// fallback
			if cached, ok := a.cache.Get(ns.Namespace); ok {
				b = cached
			} else {
				return nil, Metadata{}, fmt.Errorf("ApolloMultiSource: namespace %s load failed: %w", ns.Namespace, err)
			}
		} else {
			// 更新缓存
			a.cache.Set(ns.Namespace, b)
		}

		// 解析 namespace 内容
		var data map[string]any
		if err := json.Unmarshal(b, &data); err != nil {
			return nil, Metadata{}, err
		}
		final[ns.Namespace] = data
	}

	out, err := json.Marshal(final)
	if err != nil {
		return nil, Metadata{}, err
	}

	meta := Metadata{
		Format: "json",
		Source: a.name,
	}
	return out, meta, nil
}
