package config

// Source 用于提供原始配置内容。
// 可以是 JSON 文件、YAML 文件、env map、ETCD、Consul、Git、HTTP 配置等。
type Source interface {
	// Load 返回该 Source 的配置字节和元数据（格式等）
	Load() ([]byte, Metadata, error)
}
