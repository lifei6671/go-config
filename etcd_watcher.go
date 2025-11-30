package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdConfigWatcher 负责 watch etcd 的某个 key，当 key 发生变化时触发回调。
// 典型用法：
//
//	watcher := NewEtcdConfigWatcher(cli, "/configs/app.yaml")
//
//	// 回调内部重新调用 cfg.Load(...) 即可：
//	go watcher.Start(context.Background(), func() {
//	    // 简单策略：重新从所有 Source 加载
//	    if err := cfg.Load(fileSrc, etcdSrc, envSrc); err != nil {
//	        log.Printf("reload config failed: %v", err)
//	    }
//	})
//
// 说明：
//   - Watcher 不直接持有 Config，避免循环依赖；
//   - 回调里你可以自由选择重载方案（只 reload etcd，还是全量 reload）。
type EtcdConfigWatcher struct {
	cli *clientv3.Client
	key string

	mu     sync.Mutex
	closed bool
}

// NewEtcdConfigWatcher 创建一个新的 etcd 配置 watcher。
// cli：已经初始化好的 clientv3.Client。
// key：需要 watch 的配置 key。
func NewEtcdConfigWatcher(cli *clientv3.Client, key string) *EtcdConfigWatcher {
	return &EtcdConfigWatcher{
		cli: cli,
		key: key,
	}
}

// Start 开始监听 etcd key 的变化。
// ctx 用于整体控制生命周期；onChange 在检测到变更时被调用。
//
// 注意：
//   - Start 本身是阻塞的，通常用 goroutine 调用：go watcher.Start(...)
//   - onChange 应该是短平快的逻辑（或内部自己开 goroutine），避免阻塞 watch 循环。
func (w *EtcdConfigWatcher) Start(ctx context.Context, onChange func()) error {
	if w.cli == nil {
		return fmt.Errorf("EtcdConfigWatcher: client is nil")
	}
	if w.key == "" {
		return fmt.Errorf("EtcdConfigWatcher: key is empty")
	}
	if onChange == nil {
		return fmt.Errorf("EtcdConfigWatcher: onChange callback is nil")
	}

	// 在开始之前先检查一次当前值的存在性
	{
		ctxInit, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, err := w.cli.Get(ctxInit, w.key)
		cancel()
		if err != nil {
			return fmt.Errorf("EtcdConfigWatcher: initial get key %q failed: %w", w.key, err)
		}
	}

	ch := w.cli.Watch(ctx, w.key)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case resp, ok := <-ch:
			if !ok {
				// watch channel 关闭，通常是网络问题或 etcd 客户端关闭
				return fmt.Errorf("EtcdConfigWatcher: watch channel closed")
			}
			if resp.Err() != nil {
				// 可以按需做重试，这里先直接返回错误
				return fmt.Errorf("EtcdConfigWatcher: watch error: %w", resp.Err())
			}

			// 只要有事件（PUT/DELETE），都认为配置有变更，触发回调
			if len(resp.Events) > 0 {
				onChange()
			}
		}
	}
}
