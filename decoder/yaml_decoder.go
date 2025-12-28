package decoder

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// YAMLDecoder 支持解析 .yaml 和 .yml 文件。
// YAML 天生支持 map[interface{}]interface{}，
// 必须转换为 map[string]any，否则会导致 DefaultConfig.Merge 失败。
type YAMLDecoder struct{}

func NewYAMLDecoder() YAMLDecoder {
	return YAMLDecoder{}
}

// Format 实现 Decoder 接口。
func (YAMLDecoder) Format() string {
	return "yaml"
}

// Decode 实现 Decoder 接口，将 YAML 转 map[string]any。
func (YAMLDecoder) Decode(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml decode failed: %w", err)
	}

	// YAML => 统一转 map[string]any
	out, ok := convertYAMLToMapStringAny(raw).(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	return out, nil
}

// convertYAMLToMapStringAny 递归转换 YAML 的 map[interface{}]interface{}。
func convertYAMLToMapStringAny(v any) any {
	switch vv := v.(type) {

	case map[string]any:
		for k, val := range vv {
			vv[k] = convertYAMLToMapStringAny(val)
		}
		return vv

	case map[any]any:
		m := make(map[string]any)
		for k, val := range vv {
			ks := fmt.Sprint(k)
			m[ks] = convertYAMLToMapStringAny(val)
		}
		return m

	case []any:
		for i := range vv {
			vv[i] = convertYAMLToMapStringAny(vv[i])
		}
		return vv

	default:
		return v
	}
}
