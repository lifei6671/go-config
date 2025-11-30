package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Metadata struct {
	Format string // "json" | "yaml" | "toml" | "env"
	Source string // 文件路径、URL、标识符等
}

// Config 是整个配置系统的门面接口。
// 负责加载、合并、解析、环境变量替换、反序列化等能力。
type Config interface {
	// Load 从多个 Source 加载配置（可以是文件、内存、环境变量、远程配置中心等）。
	Load(sources ...Source) error

	// Unmarshal 将最终合并好的配置解析到结构体中（支持 json/yaml/toml 等）。
	Unmarshal(target any) error

	// Get 获取某个 key（路径），返回序列化后的 interface{}。
	Get(path string) (any, bool)

	// GetString / GetInt / GetBool 等常用方法
	GetString(path string) (string, bool)
	GetInt(path string) (int, bool)
	GetBool(path string) (bool, bool)
	GetDuration(path string) (time.Duration, bool)

	// WithEnvPrefix 设置环境变量前缀，比如 "MYAPP_"
	WithEnvPrefix(prefix string)

	// EnableEnvExpand 启用环境变量占位符替换，如 ${DB_HOST}、${PORT:8080}
	EnableEnvExpand()

	// Keys 返回整个配置的所有 key 列表，用于调试或导出。
	Keys() []string
}

// DefaultConfig 是 Config 的默认实现
type DefaultConfig struct {
	mu sync.RWMutex

	decoders map[string]Decoder
	merge    MergeStrategy

	expander  VariableExpander
	envPrefix string
	envExpand bool
	data      map[string]any
}

// 编译期检查接口实现
var _ Config = (*DefaultConfig)(nil)

// Option 模式
type Option func(*DefaultConfig)

func WithDecoder(dec Decoder) Option {
	return func(c *DefaultConfig) {
		if dec == nil {
			return
		}
		if c.decoders == nil {
			c.decoders = make(map[string]Decoder)
		}
		format := strings.ToLower(dec.Format())
		if format != "" {
			c.decoders[format] = dec
		}
	}
}

func WithMergeStrategy(ms MergeStrategy) Option {
	return func(c *DefaultConfig) {
		if ms != nil {
			c.merge = ms
		}
	}
}

func WithVariableExpander(exp VariableExpander) Option {
	return func(c *DefaultConfig) {
		if exp != nil {
			c.expander = exp
		}
	}
}

func NewDefaultConfig(opts ...Option) *DefaultConfig {
	c := &DefaultConfig{
		decoders: make(map[string]Decoder),
		merge:    DefaultMergeStrategy{},
		data:     make(map[string]any),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Load 从多个 Source 依次加载并合并配置
func (c *DefaultConfig) Load(sources ...Source) error {
	if len(sources) == 0 {
		return nil
	}

	tmp := make(map[string]any)

	for _, src := range sources {
		if src == nil {
			continue
		}
		raw, meta, err := src.Load()
		if err != nil {
			return fmt.Errorf("load source %q failed: %w", meta.Source, err)
		}

		format := strings.ToLower(meta.Format)
		dec, ok := c.decoders[format]
		if !ok {
			return fmt.Errorf("no decoder registered for format: %s", format)
		}

		m, err := dec.Decode(raw)
		if err != nil {
			return fmt.Errorf("decode source %q failed: %w", meta.Source, err)
		}

		tmp, err = c.merge.Merge(tmp, m)
		if err != nil {
			return fmt.Errorf("merge source %q failed: %w", meta.Source, err)
		}
	}

	// 环境变量占位符替换
	if c.envExpand && c.expander != nil {
		lookup := func(name string) (string, bool) {
			if c.envPrefix != "" {
				name = c.envPrefix + name
			}
			return os.LookupEnv(name)
		}
		if err := expandMapInPlace(tmp, c.expander, lookup); err != nil {
			return fmt.Errorf("expand env vars failed: %w", err)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = tmp
	return nil
}

// Unmarshal 将最终配置映射到结构体
// 实现方式: data -> JSON -> target 结构体
func (c *DefaultConfig) Unmarshal(target any) error {
	if target == nil {
		return errors.New("target is nil")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.data == nil {
		return errors.New("config is empty, call Load first")
	}

	b, err := json.Marshal(c.data)
	if err != nil {
		return fmt.Errorf("marshal config map failed: %w", err)
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("unmarshal into target failed: %w", err)
	}
	return nil
}

func (c *DefaultConfig) WithEnvPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.envPrefix = prefix
}

func (c *DefaultConfig) EnableEnvExpand() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.envExpand = true
}

func (c *DefaultConfig) Get(path string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if path == "" {
		if c.data == nil {
			return nil, false
		}
		return c.data, true
	}

	return getByPath(c.data, path)
}

func (c *DefaultConfig) GetString(path string) (string, bool) {
	v, ok := c.Get(path)
	if !ok || v == nil {
		return "", false
	}
	switch vv := v.(type) {
	case string:
		return vv, true
	case fmt.Stringer:
		return vv.String(), true
	default:
		return fmt.Sprint(v), true
	}
}

func (c *DefaultConfig) GetInt(path string) (int, bool) {
	v, ok := c.Get(path)
	if !ok || v == nil {
		return 0, false
	}
	switch vv := v.(type) {
	case int:
		return vv, true
	case int8:
		return int(vv), true
	case int16:
		return int(vv), true
	case int32:
		return int(vv), true
	case int64:
		return int(vv), true
	case uint:
		return int(vv), true
	case uint8:
		return int(vv), true
	case uint16:
		return int(vv), true
	case uint32:
		return int(vv), true
	case uint64:
		return int(vv), true
	case float32:
		return int(vv), true
	case float64:
		return int(vv), true
	case string:
		// 简单解析
		var i int
		_, err := fmt.Sscanf(vv, "%d", &i)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func (c *DefaultConfig) GetBool(path string) (bool, bool) {
	v, ok := c.Get(path)
	if !ok || v == nil {
		return false, false
	}
	switch vv := v.(type) {
	case bool:
		return vv, true
	case string:
		switch strings.ToLower(vv) {
		case "true", "1", "yes", "y", "on":
			return true, true
		case "false", "0", "no", "n", "off":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func (c *DefaultConfig) GetDuration(path string) (time.Duration, bool) {
	v, ok := c.Get(path)
	if !ok || v == nil {
		return 0, false
	}
	switch vv := v.(type) {
	case time.Duration:
		return vv, true
	case string:
		d, err := time.ParseDuration(vv)
		if err != nil {
			return 0, false
		}
		return d, true
	case int64:
		return time.Duration(vv), true
	case int:
		return time.Duration(vv), true
	case float64:
		return time.Duration(vv), true
	default:
		return 0, false
	}
}

// Keys 返回所有扁平化 key（类似 "db.host", "db.port"）
func (c *DefaultConfig) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var keys []string
	flattenKeys("", c.data, &keys)
	return keys
}
