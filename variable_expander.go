package config

import (
	"strings"
)

// DefaultVariableExpander 是配置占位符解析的默认实现。
// 支持格式：
//
//  1. 从环境变量直接读取：
//     ${env.DB_HOST}
//
//  2. 环境变量 + 默认值：
//     ${env.PORT|8080}
//
//  3. 作为字符串子串：
//     Listen="0.0.0.0:${env.PORT|8080}"
//
//  4. 基于前缀的环境变量：
//     ${env.HOST} => 读取 ENV_HOST
//     ${redis.PORT|6379} => 读取 REDIS_PORT
//
// 占位符内部格式：
//
//	${<source>.<key>[|default]}
//
// 其中：
//   - source 为 env 或 任意前缀（如 db / redis）
//   - key 为变量名
//   - default 可选，表示环境变量不存在时使用该值
//
// 示例环境：
//
//	export ENV_HOST=10.0.0.1
//
//	${env.HOST|127.0.0.1} => "10.0.0.1"
//	${env.PORT|3306}      => "3306"（默认）
type DefaultVariableExpander struct{}

// 编译期检查
var _ VariableExpander = (*DefaultVariableExpander)(nil)

// NewDefaultVariableExpander 初始默认环境变量处理逻辑
func NewDefaultVariableExpander() DefaultVariableExpander {
	return DefaultVariableExpander{}
}

// Expand 实现 VariableExpander 接口。
// lookup 参数用于从 DefaultConfig 注入环境变量读取逻辑。
func (DefaultVariableExpander) Expand(input string, lookup func(string) (string, bool)) (string, error) {
	if input == "" || !strings.Contains(input, "${") {
		return input, nil
	}

	// 扫描并替换所有占位符
	return expandAllPlaceholders(input, lookup)
}
