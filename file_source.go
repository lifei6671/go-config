package config

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// FileSource 是基于本地文件或任意 fs.FS 的配置源实现。
// 典型用途：从 JSON / YAML / TOML 等配置文件加载配置内容。
//
// FileSource 不关心文件内容如何解析，只负责：
//  1. 打开文件并读取到内存（[]byte）
//  2. 根据文件扩展名推断配置格式（json/yaml/toml）
//  3. 返回给 DefaultConfig.Load() 使用
//
// 线程安全说明：
//
//	FileSource 是只读的（创建后字段不再修改），可以安全地在多个 goroutine 中并发调用 Load()。
type FileSource struct {
	// path 是在 fs.FS 中的相对路径（或直接是操作系统路径）。
	path string

	// format 是配置格式，例如 "json" / "yaml" / "toml"。
	// 如果为空，则在 Load() 时根据文件扩展名自动推断。
	format string

	// fsys 是一个抽象的文件系统。
	// 如果 fsys == nil，则使用 os.ReadFile 直接从本地文件系统读取。
	// 如果 fsys != nil，则会通过 fsys.Open(path) 打开文件。
	fsys fs.FS

	// name 是该 Source 的标识，用于 Metadata.Source。
	// 如果为空，则默认使用 path。
	name string
}

// FileSourceOption 用于在 NewFileSource 中配置 FileSource 的可选参数。
type FileSourceOption func(*FileSource)

// WithFileSourceFormat 指定文件的配置格式。
// 常见格式：
//   - "json"
//   - "yaml"
//   - "yml"（内部会转换为 "yaml"）
//   - "toml"
//
// 一般情况下可以不设置，让 FileSource 根据扩展名自动推断。
// 但当文件没有后缀或者后缀不可信时，可以显式指定。
func WithFileSourceFormat(format string) FileSourceOption {
	return func(fs *FileSource) {
		if format == "" {
			return
		}
		fs.format = normalizeFormat(format)
	}
}

// WithFileSourceFS 指定文件系统实现，例如：
//   - os.DirFS("config")           // 从 config 目录下读相对路径
//   - embed.FS                    // Go 1.16+ 的嵌入文件系统
//   - 自定义实现 fs.FS 的虚拟文件系统
//
// 如果不设置（默认为 nil），则 FileSource 直接使用 os.ReadFile(path)。
func WithFileSourceFS(fsys fs.FS) FileSourceOption {
	return func(fs *FileSource) {
		fs.fsys = fsys
	}
}

// WithFileSourceName 指定该 Source 的展示名称，用于 Metadata.Source。
// 例如：
//   - "base-config"
//   - "local-overrides"
//   - "remote-sync"
//
// 如果不指定，默认使用 path 字符串。
func WithFileSourceName(name string) FileSourceOption {
	return func(fs *FileSource) {
		if strings.TrimSpace(name) == "" {
			return
		}
		fs.name = name
	}
}

// NewFileSource 创建一个基于文件的配置源。
//
// path 参数：
//   - 当未指定 WithFileSourceFS 时，path 会直接用于 os.ReadFile(path)
//   - 当指定了 WithFileSourceFS 时，path 会作为 fs.FS 的相对路径传入 fs.Open(path)
//
// 示例：
//
//	// 最常见：直接用本地文件系统
//	src := NewFileSource("config/app.yaml")
//
//	// 使用 os.DirFS 映射到子目录，path 写相对路径
//	src := NewFileSource("app.yaml", WithFileSourceFS(os.DirFS("config")))
//
//	// 使用 embed.FS（假设 embedFS 实现了 fs.FS）
//	src := NewFileSource("config/app.yaml", WithFileSourceFS(embedFS))
//
//	// 显式指定格式（例如文件没有后缀）
//	src := NewFileSource("config/app", WithFileSourceFormat("yaml"))
//
// 调用方通常是：
//
//	cfg := NewDefaultConfig(
//	    WithDecoder(NewJSONDecoder()),
//	    WithDecoder(NewYAMLDecoder()),
//	    WithDecoder(NewTOMLDecoder()),
//	)
//	err := cfg.Load(
//	    NewFileSource("config/base.yaml"),
//	    NewFileSource("config/local.toml"),
//	)
func NewFileSource(path string, opts ...FileSourceOption) *FileSource {
	source := &FileSource{
		path: strings.TrimSpace(path),
	}
	for _, opt := range opts {
		opt(source)
	}

	// 如果 name 未设置，则默认使用 path 作为 Source 名称
	if source.name == "" {
		source.name = source.path
	}
	return source
}

// Load 实现 Source 接口，负责：
//
//  1. 从指定文件系统读取文件内容到内存
//  2. 推断配置格式（Format）
//  3. 构造 Metadata 并返回
//
// 返回值：
//   - data: 文件的原始字节内容
//   - meta: 包含 Format 和 Source 名称的元信息
//   - err : 读取失败或格式推断失败时返回错误
func (f *FileSource) Load() ([]byte, Metadata, error) {
	if f.path == "" {
		return nil, Metadata{}, fmt.Errorf("FileSource: path is empty")
	}

	// 1. 读取文件原始内容
	data, err := f.readFile()
	if err != nil {
		return nil, Metadata{}, err
	}

	// 2. 获取格式（优先使用手动指定的 format，否则根据扩展名推断）
	format := f.format
	if format == "" {
		format, err = detectFormatFromPath(f.path)
		if err != nil {
			return nil, Metadata{}, err
		}
	}

	// 3. 构造 Metadata
	meta := Metadata{
		Format: format,
		Source: f.name,
	}
	return data, meta, nil
}

// readFile 负责真正的文件读取逻辑。
// 如果配置了 fs.FS，则使用 fsys.Open；否则使用 os.ReadFile。
func (f *FileSource) readFile() ([]byte, error) {
	// 情况一：未配置 fs.FS，直接用操作系统文件系统
	if f.fsys == nil {
		b, err := os.ReadFile(f.path)
		if err != nil {
			return nil, fmt.Errorf("FileSource: read file %q failed: %w", f.path, err)
		}
		return b, nil
	}

	// 情况二：使用抽象文件系统 fs.FS
	file, err := f.fsys.Open(f.path)
	if err != nil {
		return nil, fmt.Errorf("FileSource: open file %q from fs.FS failed: %w", f.path, err)
	}
	defer file.Close()

	b, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("FileSource: read file %q from fs.FS failed: %w", f.path, err)
	}
	return b, nil
}
