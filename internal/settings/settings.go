// Package settings 持久化应用设置（JSON，存于应用数据目录）。
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings 是用户可调整的设置项。
type Settings struct {
	// GostPath 手动指定的 gost 二进制路径，非空时优先使用。
	GostPath string `json:"gostPath"`
	// Mirror 是 gost 下载地址模板，支持 {version} {os} {arch} {ext} 变量。
	Mirror string `json:"mirror"`
	// KeepOnExit 为 true 时应用退出不停止 gost 进程。
	KeepOnExit bool `json:"keepOnExit"`
	// AutoStart 为 true 时应用启动后自动运行当前配置档。
	AutoStart bool `json:"autoStart"`
	// Current 是当前选中的配置档 ID。
	Current string `json:"current"`
}

// Store 并发安全地读写 settings.json。
type Store struct {
	mu   sync.Mutex
	path string
	s    Settings
}

// Load 从 dir/settings.json 读取，文件不存在时返回默认值。
func Load(dir string) (*Store, error) {
	st := &Store{path: filepath.Join(dir, "settings.json")}
	data, err := os.ReadFile(st.path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &st.s); err != nil {
		return nil, err
	}
	return st, nil
}

// Get 返回设置快照。
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s
}

// Update 在 fn 中修改设置并落盘。
func (st *Store) Update(fn func(*Settings)) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	fn(&st.s)
	data, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(st.path, data, 0o644)
}
