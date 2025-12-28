package decoder

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// PropertiesDecoder 适配 Java Properties 格式的解析器
// 格式示例：
//
//	# Comment
//	app.name = MyApp
//	timeout: 5000
//	path /tmp/test
//
// 特性：
//   - 支持 # 和 ! 注释
//   - 支持 "=", ":" 或空白作为 key/value 分隔符
//   - 支持反斜杠续行
//   - 支持 \uXXXX Unicode 转义
//   - 自动 trim key/value
type PropertiesDecoder struct{}

func NewPropertiesDecoder() PropertiesDecoder {
	return PropertiesDecoder{}
}

// Format 实现 Decoder 接口
func (PropertiesDecoder) Format() string {
	return "properties"
}

// Decode 将原始 Properties 文本解析为 map[string]any
func (PropertiesDecoder) Decode(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}

	m, err := ParseProperties(string(data))
	if err != nil {
		return nil, fmt.Errorf("properties decode failed: %w", err)
	}

	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out, nil
}

// ParseProperties 按 Java Properties 标准解析字符串为 map[string]string。
// 核心参考：Java java.util.Properties 的格式兼容。
func ParseProperties(input string) (map[string]string, error) {
	lines := splitLogicalLines(input)
	props := make(map[string]string)

	for _, line := range lines {
		// 去除前后空白
		trim := strings.TrimSpace(line)

		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "!") {
			continue
		}

		key, val, ok := splitKeyValue(trim)
		if !ok {
			continue
		}

		uk, err := unescape(key)
		if err != nil {
			return nil, fmt.Errorf("invalid key %q: %w", key, err)
		}

		uv, err := unescape(val)
		if err != nil {
			return nil, fmt.Errorf("invalid value for key %q: %w", uk, err)
		}

		props[uk] = uv
	}

	return props, nil
}

// splitLogicalLines 处理 Java Properties 的续行特性：
// 行尾如果是反斜杠 "\" 则续接下一行。
func splitLogicalLines(s string) []string {
	raw := strings.Split(s, "\n")
	lines := make([]string, 0, len(raw))

	var buf strings.Builder
	continuation := false

	for _, r := range raw {
		line := strings.TrimRight(r, "\r")

		if continuation {
			buf.WriteString(strings.TrimLeftFunc(line, unicode.IsSpace))
		} else {
			buf.Reset()
			buf.WriteString(strings.TrimSpace(line))
		}

		if strings.HasSuffix(buf.String(), "\\") {
			continuation = true
			// 去掉尾部反斜杠
			tmp := buf.String()
			buf.Reset()
			buf.WriteString(tmp[:len(tmp)-1])
		} else {
			continuation = false
			lines = append(lines, buf.String())
		}
	}

	if continuation {
		lines = append(lines, buf.String())
	}
	return lines
}

// splitKeyValue 按 Java 规则拆分 key/value：
// key [=|:|whitespace] value
func splitKeyValue(s string) (string, string, bool) {
	// 按 '=', ':' 轮询分隔
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '=', ':':
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
		}
	}

	// 否则按第一个空白分隔
	if idx := strings.IndexFunc(s, unicode.IsSpace); idx >= 0 {
		return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:]), true
	}

	// 整行都是 key，没有 value
	return s, "", true
}

// unescape 负责处理 Java Properties 的转义：
//
//	\n  \t  \r  \\
//	\uXXXX  (unicode)
func unescape(s string) (string, error) {
	var out strings.Builder
	runes := []rune(s)
	n := len(runes)

	for i := 0; i < n; i++ {
		c := runes[i]

		if c != '\\' {
			out.WriteRune(c)
			continue
		}

		// 碰到反斜杠
		i++
		if i >= n {
			return "", fmt.Errorf("invalid escape sequence at end of string")
		}

		c2 := runes[i]
		switch c2 {
		case 't':
			out.WriteByte('\t')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case '\\':
			out.WriteByte('\\')
		case 'u': // unicode 转义
			if i+4 >= n {
				return "", fmt.Errorf("invalid unicode escape \\uXXXX")
			}
			hex := string(runes[i+1 : i+5])
			i += 4
			val, err := strconv.ParseInt(hex, 16, 32)
			if err != nil {
				return "", fmt.Errorf("invalid unicode escape: %v", err)
			}
			out.WriteRune(rune(val))
		default:
			out.WriteRune(c2)
		}
	}

	return out.String(), nil
}
