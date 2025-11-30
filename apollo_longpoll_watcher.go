package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ApolloLongPollWatcher struct {
	BaseURL string
	AppID   string
	Cluster string

	Namespaces []ApolloNamespaceConfig
	client     *http.Client

	// release keys 用于变化检测
	releaseKeys map[string]string
}

func NewApolloLongPollWatcher(baseURL, appID, cluster string, namespaces []ApolloNamespaceConfig) *ApolloLongPollWatcher {
	rk := make(map[string]string)
	for _, ns := range namespaces {
		rk[ns.Namespace] = ""
	}
	return &ApolloLongPollWatcher{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		AppID:       appID,
		Cluster:     cluster,
		Namespaces:  namespaces,
		client:      &http.Client{Timeout: 70 * time.Second},
		releaseKeys: rk,
	}
}

// Start 使用 Apollo 长轮询通知机制
func (w *ApolloLongPollWatcher) Start(ctx context.Context, onChange func()) error {
	type nItem struct {
		NamespaceName  string `json:"namespaceName"`
		NotificationID int64  `json:"notificationId"`
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// 构造 notifications 参数
		list := []nItem{}
		for _, ns := range w.Namespaces {
			list = append(list, nItem{
				NamespaceName:  ns.Namespace,
				NotificationID: w.notificationID(ns.Namespace),
			})
		}
		payload, _ := json.Marshal(list)

		url := fmt.Sprintf(
			"%s/notifications/v2?appId=%s&cluster=%s&notifications=%s",
			w.BaseURL,
			w.AppID,
			w.Cluster,
			string(payload),
		)

		req, _ := http.NewRequest("GET", url, nil)
		resp, err := w.client.Do(req)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		if resp.StatusCode == 304 {
			continue
		}
		if resp.StatusCode != 200 {
			time.Sleep(3 * time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Apollo 返回 changed namespaces
		var changes []nItem
		if err := json.Unmarshal(body, &changes); err != nil {
			continue
		}

		// 更新 release key，触发回调
		if len(changes) > 0 {
			for _, c := range changes {
				// bump notification id
				w.setNotificationID(c.NamespaceName, c.NotificationID)
			}
			onChange()
		}
	}
}

func (w *ApolloLongPollWatcher) notificationID(ns string) int64 {
	if v, ok := w.releaseKeys[ns]; ok {
		if v == "" {
			return -1 // default
		}
	}
	return -1
}

func (w *ApolloLongPollWatcher) setNotificationID(ns string, id int64) {
	// Apollo 不强制 cache release key，这里只存一个存在性标记
	w.releaseKeys[ns] = fmt.Sprintf("%d", id)
}
