package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ApolloSource 从 Apollo 配置中心拉取指定 namespace 的配置。
// 这是一个一次性 “拉取” Source，实现 Source 接口。
// 如果你想支持热更新/自动 reload，请使用 ApolloWatcher。
type ApolloSource struct {
	// Apollo 服务基础地址，例如 "http://apollo-server:8080"
	BaseURL string

	// 应用 ID（AppId），在 Apollo 中注册的应用名
	AppID string

	// 集群名（cluster），例如 "default" 或生产环境 cluster 名
	Cluster string

	// 命名空间（namespace），例如 "application", 或自定义 namespace 名
	Namespace string

	// HTTP 客户端，可通过 Option 注入自定义 client（如带 TLS / proxy /超时等）
	client *http.Client

	// 本 Source 的逻辑名称，用于 Metadata.Source
	name string

	// Format 表示该 namespace 存储内容的格式，例如 "properties", "yaml", "json" 等。
	// 如果为空，则通过 Apollo 返回的 content-type 或 namespace 后缀推断。
	// 我们的 Decoder 支持 json/yaml/toml。如果 namespace 内容是纯 key-value (properties)，
	// 你可能需要额外一个解析器，或先转为 json/yaml。
	Format string
}

// 确保编译期实现 Source 接口
var _ Source = (*ApolloSource)(nil)

// ApolloSourceOption 用于配置 ApolloSource
type ApolloSourceOption func(*ApolloSource)

// WithApolloClient 注入自定义 http.Client
func WithApolloClient(client *http.Client) ApolloSourceOption {
	return func(a *ApolloSource) {
		if client != nil {
			a.client = client
		}
	}
}

// WithApolloSourceName 设置 Source 的逻辑名称
func WithApolloSourceName(name string) ApolloSourceOption {
	return func(a *ApolloSource) {
		if strings.TrimSpace(name) != "" {
			a.name = strings.TrimSpace(name)
		}
	}
}

// WithApolloFormat 强制指定 namespace 内容格式
func WithApolloFormat(format string) ApolloSourceOption {
	return func(a *ApolloSource) {
		if format != "" {
			a.Format = normalizeFormat(format)
		}
	}
}

// NewApolloSource 构造 ApolloSource
func NewApolloSource(baseURL, appID, cluster, namespace string, opts ...ApolloSourceOption) *ApolloSource {
	a := &ApolloSource{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		AppID:     strings.TrimSpace(appID),
		Cluster:   strings.TrimSpace(cluster),
		Namespace: strings.TrimSpace(namespace),
		client:    &http.Client{Timeout: 5 * time.Second},
		name:      "",
		Format:    "",
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.name == "" {
		a.name = fmt.Sprintf("apollo[%s:%s:%s]", a.AppID, a.Cluster, a.Namespace)
	}
	return a
}

// Load 实现 Source 接口，通过 Apollo HTTP API 拉取配置
func (a *ApolloSource) Load() ([]byte, Metadata, error) {
	if a.BaseURL == "" || a.AppID == "" || a.Cluster == "" || a.Namespace == "" {
		return nil, Metadata{}, fmt.Errorf("ApolloSource: missing parameters (BaseURL/AppID/Cluster/Namespace)")
	}

	// 拼接 Apollo 配置获取 API URL
	// 假设使用 Apollo open API: {BaseURL}/configs/{appId}/{cluster}/{namespace}
	// 例如: http://apollo-server:8080/configs/my-app/default/application
	url := fmt.Sprintf("%s/configs/%s/%s/%s", a.BaseURL, a.AppID, a.Cluster, a.Namespace)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("ApolloSource: create request failed: %w", err)
	}
	req.Header.Set("Accept", "*/*")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("ApolloSource: http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, Metadata{}, fmt.Errorf("ApolloSource: unexpected status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("ApolloSource: read body failed: %w", err)
	}

	// Apollo 返回的是一个 JSON 对象，通常结构类似：
	// {
	//   "appId": "...",
	//   "cluster": "...",
	//   "namespaceName": "...",
	//   "configurations": { "key1": "value1", "key2": "value2", ... },
	//   "releaseKey": "..."
	// }
	//
	// 这里我们需要把 configurations 块提取出来，并序列化为我们内部通用格式（json/yaml/toml map）

	var respObj struct {
		Configurations map[string]string `json:"configurations"`
	}
	if err := json.Unmarshal(body, &respObj); err != nil {
		return nil, Metadata{}, fmt.Errorf("ApolloSource: parse response JSON failed: %w", err)
	}

	// 将 configurations map[string]string 转为 map[string]any
	// 然后序列化为 JSON bytes，以兼容 JSONDecoder
	generic := make(map[string]any, len(respObj.Configurations))
	for k, v := range respObj.Configurations {
		generic[k] = v
	}

	dataBytes, err := json.Marshal(generic)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("ApolloSource: marshal configurations failed: %w", err)
	}

	meta := Metadata{
		Format: "json",
		Source: a.name,
	}
	return dataBytes, meta, nil
}
