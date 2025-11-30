package decoder

import (
	"fmt"

	toml "github.com/pelletier/go-toml/v2"
)

// TOMLDecoder 实现对 TOML 配置的解析。
// go-toml v2 解析结果格式天然为 map[string]any，不需要像 YAML 那样手工转换。
type TOMLDecoder struct{}

// Format 实现 Decoder 接口。
func (TOMLDecoder) Format() string {
	return "toml"
}

// Decode 实现 Decoder 接口，将 TOML 转 map[string]any。
func (TOMLDecoder) Decode(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}

	var out map[string]any
	if err := toml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("toml decode failed: %w", err)
	}
	return out, nil
}
