package config

// Decoder 将不同格式的配置字节解析为 map[string]any 中间层结构。
// 然后由 Config 合并多个结构、执行 env 扩展等。
type Decoder interface {
	Decode(data []byte) (map[string]any, error)
	Format() string // "json" | "yaml" | "toml" ...
}
