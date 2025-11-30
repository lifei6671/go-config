package config

import (
	"context"
	"time"
)

// ApolloWatcher 用于周期性拉取 Apollo 配置，并在发现变化时触发回调。
// 适用于不需要复杂 long-polling 的场景，如简单服务、配置不频繁变动时。
// 如果你想模仿 Apollo Java SDK 的 long-polling + local cache + 推送机制，需要更复杂实现 —— 这里仅提供“定时轮询 + reload”基础功能。
type ApolloWatcher struct {
	source *ApolloSource
	period time.Duration
}

// NewApolloWatcher 创建一个 watcher。
// period 表示每隔多久拉取一次配置，推荐 30s ~ 5min 不等。
func NewApolloWatcher(src *ApolloSource, period time.Duration) *ApolloWatcher {
	return &ApolloWatcher{source: src, period: period}
}

// Start 启动轮询监控。ctx 控制生命周期；onChange 在检测到配置变更时回调。
// 回调中通常调用 Config.Load(...) 重新加载所有 Source。
func (w *ApolloWatcher) Start(ctx context.Context, onChange func()) error {
	if w.source == nil {
		return nil
	}
	ticker := time.NewTicker(w.period)
	defer ticker.Stop()

	// lastContent 用来记录上一次加载到的 raw bytes，用于变化检测
	var lastContent []byte

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			data, _, err := w.source.Load()
			if err != nil {
				// 加载失败可以 log，但不退出 watcher
				// fmt.Printf("ApolloWatcher: load failed: %v\n", err)
				continue
			}
			if !equalBytes(lastContent, data) {
				lastContent = data
				onChange()
			}
		}
	}
}

// equalBytes 简单比较两个 byte slice 是否相等
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
