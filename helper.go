package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// getByPath 支持 "a.b.c" 形式的路径
func getByPath(data map[string]any, path string) (any, bool) {
	if data == nil {
		return nil, false
	}
	parts := strings.Split(path, ".")
	cur := any(data)

	for i, p := range parts {
		m, ok := toStringMap(cur)
		if !ok {
			return nil, false
		}
		v, ok := m[p]
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return v, true
		}
		cur = v
	}
	return nil, false
}

// toStringMap 尝试把各种 map 类型转为 map[string]any
func toStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		res := make(map[string]any, len(m))
		for k, v2 := range m {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			res[ks] = v2
		}
		return res, true
	default:
		return nil, false
	}
}

// flattenKeys 用于 Keys() 的递归展开
func flattenKeys(prefix string, v any, out *[]string) {
	m, ok := toStringMap(v)
	if !ok {
		if prefix != "" {
			*out = append(*out, prefix)
		}
		return
	}
	for k, v2 := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		_, isMap := toStringMap(v2)
		if isMap {
			flattenKeys(key, v2, out)
		} else {
			*out = append(*out, key)
		}
	}
}

// expandMapInPlace 在整个 map 结构中递归替换 string 值里的占位符
func expandMapInPlace(m map[string]any, exp VariableExpander, lookup func(string) (string, bool)) error {
	for k, v := range m {
		switch vv := v.(type) {
		case string:
			out, err := exp.Expand(vv, lookup)
			if err != nil {
				return err
			}
			m[k] = out
		case map[string]any, map[any]any:
			sub, ok := toStringMap(vv)
			if !ok {
				continue
			}
			if err := expandMapInPlace(sub, exp, lookup); err != nil {
				return err
			}
			m[k] = sub
		case []any:
			for i, item := range vv {
				switch iv := item.(type) {
				case string:
					out, err := exp.Expand(iv, lookup)
					if err != nil {
						return err
					}
					vv[i] = out
				case map[string]any, map[any]any:
					sub, ok := toStringMap(iv)
					if !ok {
						continue
					}
					if err := expandMapInPlace(sub, exp, lookup); err != nil {
						return err
					}
					vv[i] = sub
				default:
					// 其他类型忽略
				}
			}
			m[k] = vv
		default:
			// 其他原始类型忽略
		}
	}
	return nil
}

// detectFormatFromPath 根据文件路径的扩展名推断配置格式。
//   - .json -> "json"
//   - .yaml/.yml -> "yaml"
//   - .toml -> "toml"
//
// 如果扩展名未知或没有扩展名，会返回错误，提醒调用方显式配置格式。
func detectFormatFromPath(path string) (string, error) {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(path)))
	if ext == "" {
		return "", fmt.Errorf("FileSource: cannot detect format from path %q: missing file extension", path)
	}

	switch ext {
	case ".json":
		return "json", nil
	case ".yaml", ".yml":
		return "yaml", nil
	case ".toml":
		return "toml", nil
	default:
		return "", fmt.Errorf("FileSource: unsupported file extension %q in path %q", ext, path)
	}
}

// normalizeFormat 将用户传入的 format 统一为内部使用的标准格式名。
// 主要做两件事：
//  1. 去掉前后空格，并转为小写
//  2. 将 "yml" 归一为 "yaml"
func normalizeFormat(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	switch f {
	case "yml":
		return "yaml"
	default:
		return f
	}
}

// EnvValueParser 定义了环境变量值的解析函数签名。
// 输入为原始字符串，返回解析后的任意类型：
//   - bool / int / float64 / time.Duration / string 等。
type EnvValueParser func(raw string) any

// defaultEnvKeyNormalizer 是默认的 key 规范化函数。
// 当前策略：去掉首尾空格并转为小写。
//
// 这样 "DB__HOST" / "db__host" / "Db__Host" 最终都会映射成路径 ["db", "host"]。
func defaultEnvKeyNormalizer(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// splitEnvPair 将 "KEY=VALUE" 形式的字符串拆为 key 和 value。
// 返回值：key, value, ok；当字符串非法时 ok = false。
//
// 按 POSIX 约定，环境变量的形式为 KEY=VALUE：
//   - KEY 不包含 '='
//   - VALUE 可以为空，也可以包含 '='（因此只能按第一个 '=' 拆分）。
func splitEnvPair(kv string) (key, value string, ok bool) {
	idx := strings.Index(kv, "=")
	if idx <= 0 {
		// idx == 0 => key 为空；idx == -1 => 没有 '='
		return "", "", false
	}
	key = kv[:idx]
	value = kv[idx+1:]
	return key, value, true
}

// buildEnvPath 将逻辑 key 按 separator 拆分成层级路径，并执行规范化。
//
// 示例：
//
//	key = "DB__PRIMARY__HOST", separator = "__"
//	=> ["DB", "PRIMARY", "HOST"] => 经过 normalizer => ["db", "primary", "host"]
func buildEnvPath(key, separator string, normalizer func(string) string) []string {
	if separator == "" {
		// 不拆分，整个 key 当作一个层级
		normalized := normalizer(key)
		if normalized == "" {
			return nil
		}
		return []string{normalized}
	}

	rawParts := strings.Split(key, separator)
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		n := normalizer(p)
		if n == "" {
			// 空段直接跳过，避免产生多余的 "." 层级
			continue
		}
		parts = append(parts, n)
	}
	return parts
}

// insertNestedValue 将值写入嵌套 map 中。
// path 形如 ["db", "primary", "host"] => m["db"]["primary"]["host"] = value。
//
// 冲突处理策略：
//   - 如果中间层级已经存在且为 map，则递归进入该 map；
//   - 如果中间层级存在但不是 map，则直接覆写为新的 map，再继续向下。
//
// 这样可以保证“更具体”的配置（层级更深的路径）覆盖“更粗糙”的配置。
func insertNestedValue(root map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}

	cur := root
	for i, p := range path {
		if i == len(path)-1 {
			// 最后一层，直接赋值
			cur[p] = value
			return
		}

		// 不是最后一层，需要保证 cur[p] 是一个 map[string]any
		next, exists := cur[p]
		if !exists {
			// 不存在则创建新的 map
			child := make(map[string]any)
			cur[p] = child
			cur = child
			continue
		}

		// 已存在：如果是 map，则进入；否则替换为新的 map
		if m, ok := toStringMap(next); ok {
			cur = m
		} else {
			child := make(map[string]any)
			cur[p] = child
			cur = child
		}
	}
}

// defaultEnvValueParser 对环境变量值做“有限理性”的类型推断：
//
//  1. 先 trim 空格
//  2. 尝试解析为布尔（true/false/yes/no/on/off，大小写不敏感）
//  3. 尝试解析为整数（十进制 int64）
//  4. 如果包含 '.' 或 'e/E'，尝试解析为 float64
//  5. 尝试解析为 time.Duration（如 "500ms", "2s", "1h30m"）
//  6. 上述都失败则保留为原字符串
//
// 注意：
//   - 不把 "0"/"1" 当作 bool，而是解析为整数，避免歧义；
//   - 解析顺序是经过权衡的，常见场景下可以较好工作。
func defaultEnvValueParser(raw string) any {
	s := strings.TrimSpace(raw)
	if s == "" {
		// 空字符串直接返回，保留为 string
		return s
	}

	// 1. 布尔值解析（不接受 "0"/"1"，避免与整数混淆）
	lower := strings.ToLower(s)
	switch lower {
	case "true", "yes", "y", "on":
		return true
	case "false", "no", "n", "off":
		return false
	}

	// 2. 尝试解析为整数（base 10，int64 范围）
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(i)
	}

	// 3. 如果包含 '.' 或指数标记，则尝试解析为 float64
	if strings.ContainsAny(s, ".eE") {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}

	// 4. 尝试解析为 time.Duration，例如 "500ms", "2s", "1h"
	//
	// time.ParseDuration 要求必须带单位，因此不会错误地把纯数字解析为纳秒。
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}

	// 5. 回退为原始字符串
	return s
}

// ==== 一些调试/测试辅助（非必须，但便于排查问题） ====

// formatEnvDebugString 可以在调试时把 EnvSource 解析出来的 map 打印出来，
// 方便检查路径拆分和类型推断是否符合预期。
// 非核心逻辑，你可以在需要时临时调用，生产代码可以不使用。
func formatEnvDebugString(root map[string]any) string {
	// 简单地复用 Keys() + Get() 逻辑也可以，不过这里不依赖 DefaultConfig。
	var out []string
	collectEnvDebugLines("", root, &out)
	return strings.Join(out, "\n")
}

func collectEnvDebugLines(prefix string, v any, out *[]string) {
	m, ok := toStringMap(v)
	if !ok {
		if prefix != "" {
			*out = append(*out, fmt.Sprintf("%s = %#v", prefix, v))
		}
		return
	}
	for k, v2 := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if _, isMap := toStringMap(v2); isMap {
			collectEnvDebugLines(key, v2, out)
		} else {
			*out = append(*out, fmt.Sprintf("%s = %#v", key, v2))
		}
	}
}

// expandAllPlaceholders 扫描整个字符串并替换所有占位符。
// 支持嵌入形式，例如：
//
//	"Listen=${env.PORT}" 或 "0.0.0.0:${env.PORT|8080}"
func expandAllPlaceholders(input string, lookup func(string) (string, bool)) (string, error) {
	var (
		res   strings.Builder
		start int
	)

	for {
		idx := strings.Index(input[start:], "${")
		if idx == -1 {
			// no more placeholders
			res.WriteString(input[start:])
			return res.String(), nil
		}

		// 写入前面的普通内容
		res.WriteString(input[start : start+idx])

		// 寻找 "}"
		begin := start + idx + 2 // skip ${
		end := strings.Index(input[begin:], "}")
		if end == -1 {
			return "", fmt.Errorf("placeholder not closed: %q", input[start:])
		}

		// 提取占位符内部内容
		rawExpr := input[begin : begin+end]

		// 替换占位符
		expanded, err := expandSinglePlaceholder(rawExpr, lookup)
		if err != nil {
			return "", err
		}
		res.WriteString(expanded)

		// 更新扫描位置
		start = begin + end + 1
	}
}

// expandSinglePlaceholder 解析并替换 **单个** 占位符内容。
// rawExpr 不含 ${ 和 }，例如：
//
//	env.DB_HOST
//	env.PORT|8080
//	gin.HOST
//	gin.PORT|3306
func expandSinglePlaceholder(rawExpr string, lookup func(string) (string, bool)) (string, error) {
	rawExpr = strings.TrimSpace(rawExpr)
	if rawExpr == "" {
		return "", fmt.Errorf("empty placeholder expression")
	}

	// 分离默认值：source.key|default
	var (
		mainPart string
		defValue string
	)
	if strings.Contains(rawExpr, "|") {
		parts := strings.SplitN(rawExpr, "|", 2)
		mainPart = strings.TrimSpace(parts[0])
		defValue = strings.TrimSpace(parts[1])
	} else {
		mainPart = rawExpr
	}

	// 拆分 source 与 key
	dot := strings.Index(mainPart, ".")
	if dot <= 0 {
		return "", fmt.Errorf("invalid placeholder (missing dot): %q", rawExpr)
	}

	source := strings.TrimSpace(mainPart[:dot])
	key := strings.TrimSpace(mainPart[dot+1:])

	if source == "" || key == "" {
		return "", fmt.Errorf("invalid placeholder: %q", rawExpr)
	}

	// 构造真实的环境变量名
	var envName string

	if source == "env" {
		// ${env.VAR} => 读取 VAR
		envName = key
	} else {
		// ${gin.HOST} => 读取 GIN_HOST
		// ${redis.PORT|6379} => 读取 REDIS_PORT
		envName = strings.ToUpper(source) + "_" + strings.ToUpper(key)
	}

	// 环境变量查找
	val, ok := lookup(envName)
	if !ok {
		// 对应环境变量未定义
		if defValue != "" {
			return defValue, nil
		}
		return "", nil
	}

	return val, nil
}

// detectFormatFromContentType 根据 Content-Type 推断配置格式。
// 常见映射：
//
//	application/json           -> json
//	application/x-yaml         -> yaml
//	text/yaml                  -> yaml
//	application/toml           -> toml
//	text/x-toml                -> toml
func detectFormatFromContentType(ct string) (string, bool) {
	ct = strings.ToLower(ct)
	if i := strings.Index(ct, ";"); i >= 0 {
		// 去掉 charset 等参数
		ct = strings.TrimSpace(ct[:i])
	}

	switch ct {
	case "application/json":
		return "json", true
	case "application/x-yaml", "text/yaml", "text/x-yaml":
		return "yaml", true
	case "application/toml", "text/x-toml":
		return "toml", true
	default:
		return "", false
	}
}
