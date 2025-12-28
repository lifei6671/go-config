package decoder

import (
	"encoding/json"
	"fmt"
)

// JSONDecoder 实现 JSON 配置解析。
// 解析结果为 map[string]any，用于与其他来源合并。
//
// 注意：
//   - JSON 数字默认是 float64，这在实际使用中通常可以接受。
//   - 如果需要严格数字类型，可以在 Merge 阶段或 Unmarshal 阶段转换。
type JSONDecoder struct{}

func NewJSONDecoder() JSONDecoder {
	return JSONDecoder{}
}

// Format 实现 Decoder 接口。
func (JSONDecoder) Format() string {
	return "json"
}

// Decode 实现 Decoder 接口，将 JSON 转 map[string]any。
func (JSONDecoder) Decode(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("json decode failed: %w", err)
	}
	return out, nil
}
