package config

type MergeStrategy interface {
	Merge(dst, src map[string]any) (map[string]any, error)
}

// DefaultMergeStrategy 简单深度合并，后加载的 source 覆盖前面的同名 key
type DefaultMergeStrategy struct{}

func (DefaultMergeStrategy) Merge(dst, src map[string]any) (map[string]any, error) {
	if dst == nil {
		dst = make(map[string]any)
	}
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			// 如果双方都是 map，则递归合并
			m1, ok1 := toStringMap(existing)
			m2, ok2 := toStringMap(v)
			if ok1 && ok2 {
				merged, err := (DefaultMergeStrategy{}).Merge(m1, m2)
				if err != nil {
					return nil, err
				}
				dst[k] = merged
				continue
			}
		}
		// 其他情况直接覆盖
		dst[k] = v
	}
	return dst, nil
}
