package config

import (
	"context"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdSource 是基于 etcd 的配置 Source 实现。
// 用途：将 etcd 中某个 key 的值作为一份配置数据来源，参与整体合并。
//
// 设计哲学：
//   - Source 只负责“读一次”：Get 指定 key，返回字节内容 + Format。
//   - 不在 Source 内部耦合 Watch / 重载逻辑，把“配置同步”做成独立组件（EtcdConfigWatcher）。
//
// 典型用法：
//
//	cli, err := clientv3.New(clientv3.Config{
//	    Endpoints:   []string{"127.0.0.1:2379"},
//	    DialTimeout: 5 * time.Second,
//	})
//	if err != nil {
//	    panic(err)
//	}
//
//	etcdSrc := NewEtcdSource(cli, "/configs/app.yaml",
//	    WithEtcdSourceFormat("yaml"),           // 或者留空，让它根据 key 后缀推断
//	    WithEtcdSourceName("etcd-app-config"),  // 用于元信息展示
//	)
//
//	cfg := NewDefaultConfig(
//	    WithDecoder(JSONDecoder{}),
//	    WithDecoder(YAMLDecoder{}),
//	    WithDecoder(TOMLDecoder{}),
//	    WithVariableExpander(DefaultVariableExpander{}),
//	)
//
//	if err := cfg.Load(
//	    NewFileSource("config/local.yaml"),
//	    etcdSrc, // etcd 中的配置覆盖本地配置
//	); err != nil {
//	    panic(err)
//	}
type EtcdSource struct {
	cli    *clientv3.Client
	key    string
	format string
	name   string

	// 读操作的超时时间，避免 etcd 请求永久阻塞。
	// 如果为 0，则使用默认超时（3 秒）。
	readTimeout time.Duration
}

// 编译期检查
var _ Source = (*EtcdSource)(nil)

// EtcdSourceOption 用于配置 EtcdSource。
type EtcdSourceOption func(*EtcdSource)

// WithEtcdSourceFormat 显式指定配置格式（"json" / "yaml" / "toml"）。
// 一般情况可以不指定，EtcdSource 会尝试根据 key 的后缀推断：
//   - /config/app.json => json
//   - /config/app.yaml => yaml
//   - /config/app.toml => toml
func WithEtcdSourceFormat(format string) EtcdSourceOption {
	return func(es *EtcdSource) {
		if format == "" {
			return
		}
		es.format = normalizeFormat(format)
	}
}

// WithEtcdSourceName 设置该 Source 的逻辑名称，用于 Metadata.Source。
// 仅影响日志/调试信息，不影响实际配置内容。
func WithEtcdSourceName(name string) EtcdSourceOption {
	return func(es *EtcdSource) {
		if strings.TrimSpace(name) == "" {
			return
		}
		es.name = name
	}
}

// WithEtcdSourceReadTimeout 设置 etcd Get 操作的超时时间。
// 默认 3 秒，避免因为网络异常导致配置加载 hang 死。
func WithEtcdSourceReadTimeout(d time.Duration) EtcdSourceOption {
	return func(es *EtcdSource) {
		if d <= 0 {
			return
		}
		es.readTimeout = d
	}
}

// NewEtcdSource 创建一个基于 etcd 的配置源。
// cli 由调用方负责创建和管理（包括关闭），EtcdSource 只复用这个客户端。
func NewEtcdSource(cli *clientv3.Client, key string, opts ...EtcdSourceOption) *EtcdSource {
	es := &EtcdSource{
		cli:         cli,
		key:         strings.TrimSpace(key),
		format:      "", // 默认留空，后续按 key 后缀推断
		name:        "", // 默认晚点用 key 填充
		readTimeout: 3 * time.Second,
	}
	for _, opt := range opts {
		opt(es)
	}
	if es.name == "" {
		es.name = es.key
	}
	return es
}

// Load 实现 Source 接口：从 etcd 读取一个 key 的 value 作为配置内容。
// 返回：value 字节内容 + Metadata{Format, Source}。
func (es *EtcdSource) Load() ([]byte, Metadata, error) {
	if es.cli == nil {
		return nil, Metadata{}, fmt.Errorf("EtcdSource: client is nil")
	}
	if es.key == "" {
		return nil, Metadata{}, fmt.Errorf("EtcdSource: key is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), es.readTimeout)
	defer cancel()

	resp, err := es.cli.Get(ctx, es.key)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("EtcdSource: get key %q failed: %w", es.key, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, Metadata{}, fmt.Errorf("EtcdSource: key %q not found", es.key)
	}

	// etcd 支持同一个 key 多个版本，这里取最后一个版本。
	kv := resp.Kvs[len(resp.Kvs)-1]
	data := kv.Value

	// 解析格式
	format := es.format
	if format == "" {
		// 尝试从 key 名推断后缀
		f, err := detectFormatFromPath(es.key)
		if err != nil {
			return nil, Metadata{}, fmt.Errorf("EtcdSource: detect format from key %q failed: %w", es.key, err)
		}
		format = f
	}

	meta := Metadata{
		Format: format,
		Source: es.name,
	}
	return data, meta, nil
}
