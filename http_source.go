package config

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPSource 是一个基于 HTTP(S) 的配置 Source 实现。
// 核心职责：发起 HTTP 请求，读取响应 body，推断格式并返回。
//
// 典型用法：
//
//	httpSrc := NewHTTPSource("https://example.com/config/app.yaml",
//	    WithHTTPSourceName("remote-app-config"),
//	    WithHTTPSourceTimeout(5 * time.Second),
//	)
//
//	cfg := NewDefaultConfig(
//	    WithDecoder(JSONDecoder{}),
//	    WithDecoder(YAMLDecoder{}),
//	    WithDecoder(TOMLDecoder{}),
//	)
//
//	if err := cfg.Load(httpSrc); err != nil {
//	    panic(err)
//	}
type HTTPSource struct {
	url     string
	method  string
	headers map[string]string
	format  string
	name    string
	client  *http.Client
}

// 编译期检查
var _ Source = (*HTTPSource)(nil)

// HTTPSourceOption 用于配置 HTTPSource。
type HTTPSourceOption func(*HTTPSource)

// WithHTTPSourceMethod 设置 HTTP 方法，默认 "GET"。
// 一般配置拉取使用 GET 即可。
func WithHTTPSourceMethod(method string) HTTPSourceOption {
	return func(hs *HTTPSource) {
		if strings.TrimSpace(method) == "" {
			return
		}
		hs.method = strings.ToUpper(method)
	}
}

// WithHTTPSourceHeader 添加自定义请求头，例如鉴权 Token 等。
func WithHTTPSourceHeader(key, value string) HTTPSourceOption {
	return func(hs *HTTPSource) {
		if hs.headers == nil {
			hs.headers = make(map[string]string)
		}
		hs.headers[key] = value
	}
}

// WithHTTPSourceFormat 显式指定配置格式，绕过自动推断。
func WithHTTPSourceFormat(format string) HTTPSourceOption {
	return func(hs *HTTPSource) {
		if format == "" {
			return
		}
		hs.format = normalizeFormat(format)
	}
}

// WithHTTPSourceName 设置该 Source 的逻辑名称，用于 Metadata.Source。
func WithHTTPSourceName(name string) HTTPSourceOption {
	return func(hs *HTTPSource) {
		if strings.TrimSpace(name) == "" {
			return
		}
		hs.name = name
	}
}

// WithHTTPSourceClient 注入自定义 http.Client，例如带代理/自定义 Transport 等。
// 如果不设置，会创建一个带超时的默认 client。
func WithHTTPSourceClient(client *http.Client) HTTPSourceOption {
	return func(hs *HTTPSource) {
		if client != nil {
			hs.client = client
		}
	}
}

// WithHTTPSourceTimeout 设置默认 client 的超时时间。
// 前提是你没有使用 WithHTTPSourceClient 自定义 client。
func WithHTTPSourceTimeout(d time.Duration) HTTPSourceOption {
	return func(hs *HTTPSource) {
		if hs.client == nil && d > 0 {
			hs.client = &http.Client{
				Timeout: d,
			}
		}
	}
}

// NewHTTPSource 创建一个基于 HTTP 的配置源。
// urlStr 必须是合法的 HTTP/HTTPS URL。
func NewHTTPSource(urlStr string, opts ...HTTPSourceOption) *HTTPSource {
	hs := &HTTPSource{
		url:    strings.TrimSpace(urlStr),
		method: http.MethodGet,
	}
	for _, opt := range opts {
		opt(hs)
	}
	if hs.name == "" {
		hs.name = hs.url
	}
	if hs.client == nil {
		hs.client = &http.Client{
			Timeout: 5 * time.Second,
		}
	}
	return hs
}

// Load 实现 Source 接口：发起 HTTP 请求，返回 body + 元数据。
func (hs *HTTPSource) Load() ([]byte, Metadata, error) {
	if hs.url == "" {
		return nil, Metadata{}, fmt.Errorf("HTTPSource: url is empty")
	}
	if _, err := url.ParseRequestURI(hs.url); err != nil {
		return nil, Metadata{}, fmt.Errorf("HTTPSource: invalid url %q: %w", hs.url, err)
	}

	req, err := http.NewRequest(hs.method, hs.url, nil)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("HTTPSource: new request failed: %w", err)
	}
	for k, v := range hs.headers {
		req.Header.Set(k, v)
	}

	resp, err := hs.client.Do(req)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("HTTPSource: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, Metadata{}, fmt.Errorf("HTTPSource: non-2xx status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("HTTPSource: read response body failed: %w", err)
	}

	// 推断格式：优先显式指定，其次 URL 后缀，最后 Content-Type。
	format := hs.format
	if format == "" {
		if f, err := detectFormatFromPath(hs.url); err == nil {
			format = f
		} else {
			// 尝试从 Content-Type 里推断
			if ct := resp.Header.Get("Content-Type"); ct != "" {
				if f, ok := detectFormatFromContentType(ct); ok {
					format = f
				}
			}
		}
	}
	if format == "" {
		return nil, Metadata{}, fmt.Errorf("HTTPSource: cannot detect format from url/content-type (url=%q)", hs.url)
	}

	meta := Metadata{
		Format: format,
		Source: hs.name,
	}
	return body, meta, nil
}
