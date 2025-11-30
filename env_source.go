package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EnvSource 是一个基于环境变量的配置源实现，满足两个典型诉求：
//
//  1. 把环境变量当作一份“覆盖层”配置（覆盖文件中的配置项）
//  2. 支持通过命名约定，把扁平的环境变量映射到层级结构中（如 DB__HOST => db.host）
//
// 设计要点：
//  1. 通过 prefix 过滤出属于当前应用的环境变量（例如 "MYAPP_"）
//  2. 通过 separator（默认 "__"）把 key 拆成层级路径
//  3. 对值做简单类型推断（int/bool/float/duration），避免 Unmarshal 时类型不匹配
//  4. 最终构造出 map[string]any，然后序列化为 JSON 字节；Metadata.Format 固定为 "json"
//     这样就复用已有的 JSON Decoder，不需要额外实现 EnvDecoder。
//
// 注意：EnvSource 只负责“从环境变量加载并生成配置结构”，
// 替换占位符（如 ${DB_HOST}）的工作由 VariableExpander + DefaultConfig 完成。
type EnvSource struct {
	// prefix 用于筛选环境变量，只有以 prefix 开头的才会被纳入配置。
	// 比如 prefix = "MYAPP_"，则只处理形如 "MYAPP_DB_HOST" 的变量。
	// 如果 prefix 为空，表示不过滤，所有环境变量都处理（一般不推荐）。
	prefix string

	// stripPrefix 表示在生成配置 key 时，是否移除 prefix。
	// 例如环境变量：MYAPP_DB__HOST
	//   - prefix = "MYAPP_"
	//   - stripPrefix = true  => 逻辑 key: "DB__HOST"
	//   - stripPrefix = false => 逻辑 key: "MYAPP_DB__HOST"
	stripPrefix bool

	// separator 用于把逻辑 key 拆成层级路径，默认建议使用 "__"（两个下划线），
	// 这样单个下划线仍可保留在 key 内，不会误拆。
	//
	// 例如：DB__PRIMARY__HOST => ["db", "primary", "host"] => db.primary.host
	separator string

	// name 是该 Source 的逻辑名称，用于 Metadata.Source。
	// 默认值为 "env"，可以通过 Option 手动修改，用于日志或调试。
	name string

	// environ 用于获取环境变量列表，默认是 os.Environ。
	// 抽象成函数是为了便于单元测试，测试时可以注入 fake 环境变量。
	environ func() []string

	// valueParser 用于将原始字符串解析为合适的 Go 类型，例如：
	//   "true" => bool
	//   "8080" => int
	//   "1.5"  => float64
	//   "500ms" => time.Duration
	//   解析失败则退回 string。
	valueParser EnvValueParser

	// keyNormalizer 用于规范化 key，默认行为是 strings.ToLower。
	// 这样环境变量 "DB__HOST" 会被映射为 "db.host"，与大部分配置文件风格统一。
	keyNormalizer func(string) string
}

// 编译期保证 EnvSource 实现了 Source 接口。
var _ Source = (*EnvSource)(nil)

// EnvSourceOption 是 EnvSource 的配置 Option。
type EnvSourceOption func(*EnvSource)

// WithEnvSourcePrefix 设置环境变量前缀。
// 通常会与 Config.WithEnvPrefix 保持一致，但两者不强制关联：
//   - EnvSource 用于“加载/覆盖配置”
//   - Config.WithEnvPrefix + VariableExpander 用于占位符替换。
func WithEnvSourcePrefix(prefix string) EnvSourceOption {
	return func(es *EnvSource) {
		es.prefix = prefix
	}
}

// WithEnvSourceStripPrefix 控制是否在构建配置 key 时移除前缀。
// 一般推荐开启，这样环境变量不会在 key 中残留 prefix。
func WithEnvSourceStripPrefix(strip bool) EnvSourceOption {
	return func(es *EnvSource) {
		es.stripPrefix = strip
	}
}

// WithEnvSourceSeparator 设置层级分隔符，默认建议使用 "__"。
//
// 常见约定示例：
//
//	MYAPP_DB__HOST => db.host
//	MYAPP_HTTP__PORT => http.port
func WithEnvSourceSeparator(sep string) EnvSourceOption {
	return func(es *EnvSource) {
		if strings.TrimSpace(sep) == "" {
			return
		}
		es.separator = sep
	}
}

// WithEnvSourceName 设置该 Source 的逻辑名称，用于 Metadata.Source。
// 仅影响日志/调试信息，不影响实际配置数据。
func WithEnvSourceName(name string) EnvSourceOption {
	return func(es *EnvSource) {
		if strings.TrimSpace(name) == "" {
			return
		}
		es.name = name
	}
}

// WithEnvSourceEnviron 注入一个自定义的环境变量获取函数。
// 主要用于单元测试，例如：
//
//	fakeEnviron := func() []string {
//	    return []string{"MYAPP_DB__HOST=127.0.0.1", "MYAPP_DB__PORT=3306"}
//	}
//	src := NewEnvSource(WithEnvSourceEnviron(fakeEnviron))
func WithEnvSourceEnviron(fn func() []string) EnvSourceOption {
	return func(es *EnvSource) {
		if fn != nil {
			es.environ = fn
		}
	}
}

// WithEnvSourceValueParser 自定义环境变量值的解析逻辑。
// 如果默认的类型推断（bool/int/float/duration）不符合需求，可以用该 Option 替换。
func WithEnvSourceValueParser(parser EnvValueParser) EnvSourceOption {
	return func(es *EnvSource) {
		if parser != nil {
			es.valueParser = parser
		}
	}
}

// WithEnvSourceKeyNormalizer 自定义 key 规范化逻辑。
// 默认实现会将 key 转为小写，例如 "DB" => "db"。
// 如果你希望保持大小写敏感，可传入一个 no-op 函数：
//
//	WithEnvSourceKeyNormalizer(func(s string) string { return s })
func WithEnvSourceKeyNormalizer(fn func(string) string) EnvSourceOption {
	return func(es *EnvSource) {
		if fn != nil {
			es.keyNormalizer = fn
		}
	}
}

// NewEnvSource 创建一个基于环境变量的配置源。
//
// 常见用法：
//
//	// 只加载以 MYAPP_ 开头的环境变量，并使用 "__" 作为层级分隔符
//	src := NewEnvSource(
//	    WithEnvSourcePrefix("MYAPP_"),
//	    WithEnvSourceStripPrefix(true),
//	    WithEnvSourceSeparator("__"),
//	)
//
//	cfg := NewDefaultConfig(
//	    WithDecoder(NewJSONDecoder()),
//	    WithDecoder(NewYAMLDecoder()),
//	    WithDecoder(NewTOMLDecoder()),
//	)
//	err := cfg.Load(
//	    NewFileSource("config/base.yaml"),
//	    src, // 环境变量作为最后一层覆盖
//	)
func NewEnvSource(opts ...EnvSourceOption) *EnvSource {
	es := &EnvSource{
		// 默认不过滤前缀，但生产场景建议使用 WithEnvSourcePrefix 进行限定
		prefix: "",
		// 默认不强制 strip 前缀，考虑到有些人可能希望保留 prefix 信息
		stripPrefix: false,
		// 默认使用 "__" 作为层级分隔符（比单下划线更安全）
		separator: "__",
		// 默认名称为 "env"
		name: "env",
		// 默认使用真实环境变量
		environ: os.Environ,
		// 默认值解析器和 key 规范化逻辑在工具文件中实现
		valueParser:   defaultEnvValueParser,
		keyNormalizer: defaultEnvKeyNormalizer,
	}

	for _, opt := range opts {
		opt(es)
	}
	return es
}

// Load 实现 Source 接口，负责：
//
//  1. 从 environ() 中获取所有环境变量（形式为 "KEY=VALUE" 的字符串）
//  2. 按 prefix 过滤、按 separator 拆分为层级路径
//  3. 对 value 做类型推断并写入 map[string]any
//  4. 将最终 map 序列化为 JSON 字节，并返回 Metadata.Format = "json"
//
// 之所以选择“序列化为 JSON 再交给 JSONDecoder”这一方案，原因是：
//   - EnvSource 自己已经构造出 map 结构，无需额外的 Decoder；
//   - 但 DefaultConfig 约定所有 Source 的数据都要通过 Decoder 走统一流程；
//   - 通过 JSON 可以复用已有 JSONDecoder，避免额外定义 "env" 格式的 Decoder。
//
// 注意：EnvSource 只会覆盖它生成的 key，合并规则由 MergeStrategy 决定
// （默认策略是后加载的 Source 覆盖前面的同名 key）。
func (es *EnvSource) Load() ([]byte, Metadata, error) {
	envList := es.environ()
	root := make(map[string]any)

	for _, kv := range envList {
		key, val, ok := splitEnvPair(kv)
		if !ok {
			// 非法的环境变量字符串（没有 "="），忽略即可
			continue
		}

		// 前缀过滤：如果设置了 prefix 且 key 不以 prefix 开头，则跳过
		if es.prefix != "" && !strings.HasPrefix(key, es.prefix) {
			continue
		}

		// 根据 stripPrefix 决定是否去掉前缀
		if es.prefix != "" && es.stripPrefix {
			key = strings.TrimPrefix(key, es.prefix)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			// key 为空没有意义，直接跳过
			continue
		}

		// 拆分层级路径并做规范化（例如转为小写）
		path := buildEnvPath(key, es.separator, es.keyNormalizer)
		if len(path) == 0 {
			// 全是空段的情况（例如 prefix 刚好等于原 key），跳过
			continue
		}

		// 对 value 做类型推断（int/bool/float/duration），解析失败时保留原字符串
		parsed := es.valueParser(val)

		// 写入到 root map 中，如果存在冲突，后者覆盖前者
		insertNestedValue(root, path, parsed)
	}

	// 将 map 序列化为 JSON，交给 JSONDecoder 使用
	data, err := json.Marshal(root)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("EnvSource: marshal env map to json failed: %w", err)
	}

	meta := Metadata{
		Format: "json", // 必须确保注册了 json 对应的 Decoder
		Source: es.name,
	}
	return data, meta, nil
}
