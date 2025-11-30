# Go-Config

高性能、可扩展、生产级的 Go 配置中心框架

Go-Config 是一个面向生产环境的**多源配置加载框架**，支持文件、多格式解析、环境变量、HTTP、ETCD、Apollo 等多种配置源，具备占位符替换、深度合并、热更新、格式解码器扩展等能力。

其核心目标是：  
**提供一个比 Viper 更简单、更干净、更可控，同时支持“云原生 + 配置中心”场景的 Go 配置体系。**

---

## 特性

### ✔ 多源配置加载  
支持同时加载多个配置来源（Source）并自动深度合并：

- 本地文件（json/yaml/yml/toml/properties）
- 环境变量
- Remote HTTP 配置
- Etcd 配置中心
- Apollo 配置中心（multi-namespace + 长轮询 + 缓存 fallback）
- 自定义 Source

### ✔ 多格式解码（Decoder）
框架内置解析器支持：

- JSON
- YAML
- TOML
- Java Properties（完全兼容 Java Properties 标准，含续行与 unicode）

可根据需要无限扩展自定义解析器。

### ✔ 深度合并（MergeStrategy）
多个源按顺序合并，后者覆盖前者，并支持 map 递归合并。

### ✔ 占位符（Variable Expand）
支持强大的占位符替换语法：

```

${env.HOST}
${env.PORT|8080}
${gdp.HOST}
Listen="0.0.0.0:${env.LISTEN|8080}"

````

规则：

- `${env.VAR}` 直接从环境变量读取 `VAR`
- `${prefix.KEY}` 读取 `${PREFIX_KEY}`
- 默认值：`${env.VAR|default}`
- 可以嵌入字符串中解析

### ✔ 热更新（Watchers）
支持多种 Watcher：

- Etcd 变更监听（watch）
- Apollo 长轮询通知（/notifications/v2）
- 文件系统 watch（可选）
- 自定义 watcher

Watcher 会触发回调 → 自动调用 Config.Load 重载配置。

### ✔ 干净的架构设计
核心接口只有三个：

```go
type Source interface {
    Load() ([]byte, Metadata, error)
}

type Decoder interface {
    Decode([]byte) (map[string]any, error)
    Format() string
}

type Config interface {
    Load(...Source) error
    Unmarshal(any) error
    Get(path string) (any, bool)
}
```

**Source 负责提供原始配置字节**
**Decoder 负责解析格式**
**Config 负责 orchestrate（合并、展开、反序列化）**

让代码保持极度清晰、可测试、可扩展。

---

## 快速上手

### 安装

```bash
go get github.com/lifei6671/go-config
```

---

## 示例：加载多源配置

```go
cfg := NewDefaultConfig(
    WithDecoder(JSONDecoder{}),
    WithDecoder(YAMLDecoder{}),
    WithDecoder(TOMLDecoder{}),
    WithDecoder(PropertiesDecoder{}),
    WithVariableExpander(DefaultVariableExpander{}),
)
cfg.EnableEnvExpand()

err := cfg.Load(
    NewFileSource("config/base.yaml"),
    NewFileSource("config/application.properties"),
    NewEnvSource(WithEnvSourcePrefix("APP_")),
    NewHTTPSource("https://config.example.com/app.yaml"),
)
if err != nil {
    panic(err)
}

type AppConfig struct {
    Server struct {
        Host string `json:"host"`
        Port int    `json:"port"`
    } `json:"server"`
}

var ac AppConfig
cfg.Unmarshal(&ac)
```

---

## 配置占位符

文件内容：

```yaml
server:
  host: ${env.HOST|127.0.0.1}
  port: ${env.PORT|8080}
  address: "0.0.0.0:${gdp.PORT|9000}"
```

所有占位符在 Load 阶段自动替换。

---

## Etcd Source & Watcher

```go
cli, _ := clientv3.New(...)
etcdSrc := NewEtcdSource(cli, "/configs/app.yaml")

cfg.Load(etcdSrc)

watcher := NewEtcdConfigWatcher(cli, "/configs/app.yaml")
go watcher.Start(context.Background(), func() {
    _ = cfg.Load(etcdSrc)
})
```

---

## Apollo 配置中心（支持多 namespace + 长轮询 + fallback）

### Source（加载配置）

```go
apolloSrc := NewApolloMultiSource(
    "http://apollo-server:8080",
    "my-app",
    "default",
    []ApolloNamespaceConfig{
        {Namespace: "application"},
        {Namespace: "db.yaml"},
    },
)

cfg.Load(apolloSrc)
```

### Watcher（长轮询通知）

```go
watcher := NewApolloLongPollWatcher(
    "http://apollo-server:8080",
    "my-app",
    "default",
    []ApolloNamespaceConfig{
        {Namespace: "application"},
        {Namespace: "db.yaml"},
    },
)

go watcher.Start(context.Background(), func() {
    _ = cfg.Load(apolloSrc)
})
```

轻松构建一个 **Apollo+本地+环境变量+HTTP 的多层合并配置体系**。

---

## Properties 支持（Java）

文件：

```
app.name = MyApp
timeout=5000
message = hello\u4F60\u597D
path = C:\\test\\file
```

自动解析为：

```json
{
  "app.name": "MyApp",
  "timeout": "5000",
  "message": "hello你好",
  "path": "C:\\test\\file"
}
```

---

## 性能特性

* 所有合并操作基于 map[string]any 深度合并
* Source/Decoder 分离保证解析与加载解耦
* 线程安全
* 支持用户自定义优化（如缓存、增量更新）

---

## License

MIT License

---

欢迎提交 Issue / PR，一起打造最现代化的 Go 配置库！

