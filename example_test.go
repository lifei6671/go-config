package config_test

import (
	"fmt"
	"os"

	"github.com/lifei6671/go-config"
	"github.com/lifei6671/go-config/decoder"
)

// ExampleNewDefaultConfig 默认配置使用示例。
// 该示例展示如何：
//  1. 创建 DefaultConfig
//  2. 注册解码器、变量扩展器
//  3. 加载多种配置来源：文件、Env、HTTP
//  4. 反序列化结构体
//  5. 使用 cfg.Get()
func ExampleNewDefaultConfig() {
	// 模拟环境变量（真实项目中在部署层注入即可）
	_ = os.Setenv("APP_SERVER_PORT", "9090")

	// 创建默认配置实例
	cfg := config.NewDefaultConfig(
		config.WithDecoder(decoder.YAMLDecoder{}),
	)
	cfg.EnableEnvExpand()

	// 从多个源加载配置
	err := cfg.Load(
		config.NewFileSource("testdata/base.yaml"),
	)
	if err != nil {
		panic(err)
	}

	// 定义配置对象
	type AppConfig struct {
		Server struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"server"`

		Log struct {
			Level string `json:"level"`
		} `json:"log"`
	}

	// Unmarshal 为结构体
	var ac AppConfig
	_ = cfg.Unmarshal(&ac)

	// 使用 Config.Get 读取动态路径
	hostValue, _ := cfg.Get("server.host")

	fmt.Println(ac.Server.Port == 9090) // 来自 APP_SERVER_PORT
	fmt.Println(hostValue != "")        // 来自文件 或 HTTP 合并结果
	fmt.Println(ac.Log.Level != "")     // 证明嵌套结构也被解析了

	// Output:
	// true
	// true
	// true
}
