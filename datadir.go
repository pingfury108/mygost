package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ─── 数据目录：默认位置、自定义指针、迁移 ──────────────────────────
//
// 默认目录是系统的用户配置目录（Windows %APPDATA%\mygost，macOS
// ~/Library/Application Support/mygost）。用户可以在设置里改成别的盘，
// 选择记在一个指针文件里。
//
// 指针文件不得不放在默认目录：settings.json 本身就在数据目录内，
// 没法用它记录"数据目录在哪"。指针只有用户改过目录后才存在，
// 内容是自定义路径的纯文本，几十字节。

// pointerName 是指针文件名；dev 版用 -dev 后缀，两边的自定义目录互不影响。
func pointerName() string {
	if pointerIsDev() {
		return "datadir-dev"
	}
	return "datadir"
}

// pointerIsDev 与 dataDirName 的 dev 判定保持一致。
func pointerIsDev() bool { return dataDirName() == "mygost-dev" }

// systemConfigBase 返回系统用户配置根目录（不含应用名）。
func systemConfigBase() string {
	if base, err := os.UserConfigDir(); err == nil {
		return base
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// defaultDataDir 返回默认数据目录（不含自定义覆盖）。
func defaultDataDir() string {
	return filepath.Join(systemConfigBase(), dataDirName())
}

// pointerPath 返回指针文件路径（位于默认目录内）。
func pointerPath() string { return filepath.Join(defaultDataDir(), pointerName()) }

// readPointer 读取自定义目录；无指针或内容无效时返回空。
func readPointer() string {
	b, err := os.ReadFile(pointerPath())
	if err != nil {
		return ""
	}
	v := cleanDirEnv(string(b))
	if v == "" || !filepath.IsAbs(v) {
		return ""
	}
	return v
}

// writePointer 原子写入自定义目录。
func writePointer(dir string) error {
	dir = cleanDirEnv(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return fmt.Errorf("目录必须是绝对路径: %q", dir)
	}
	base := defaultDataDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	tmp := pointerPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(dir+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, pointerPath())
}

// clearPointer 恢复默认目录。
func clearPointer() error {
	err := os.Remove(pointerPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ensureWritableDir 确认目录存在且可写（必要时创建）。
func ensureWritableDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("目录为空")
	}
	if st, err := os.Stat(dir); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("%s 不是目录", dir)
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	// 真正试写一次：只读挂载、权限不足、磁盘满都能在这里暴露
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return fmt.Errorf("目录不可写: %w", err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}

// validateDataDirTarget 检查候选目录是否可接受。
func validateDataDirTarget(cur, next string) error {
	next = cleanDirEnv(next)
	if next == "" {
		return fmt.Errorf("请选择一个目录")
	}
	if !filepath.IsAbs(next) {
		return fmt.Errorf("请使用绝对路径")
	}
	absCur, _ := filepath.Abs(cur)
	absNext, _ := filepath.Abs(next)
	if absCur == absNext {
		return fmt.Errorf("已经是当前目录")
	}
	// 不允许选当前目录的子目录：迁移会自我递归
	if rel, err := filepath.Rel(absCur, absNext); err == nil &&
		!strings.HasPrefix(rel, "..") && rel != "." {
		return fmt.Errorf("不能选择当前数据目录的子目录")
	}
	return ensureWritableDir(absNext)
}

// migrateDataDir 把 from 的内容复制到 to（不删除原目录）。
//
// 跳过 state/：里面是进程 PID 文件，指向原来的配置路径，
// 搬过去只会误导接管逻辑。
func migrateDataDir(from, to string) (copied int, err error) {
	if _, err := os.Stat(from); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil // 旧目录还没数据，没什么可搬的
		}
		return 0, err
	}
	err = filepath.WalkDir(from, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(from, path)
		if err != nil || rel == "." {
			return nil
		}
		// 跳过进程状态目录
		if rel == "state" {
			return filepath.SkipDir
		}
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		// 目标已存在则跳过：不覆盖用户在新目录里已有的数据
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		if err := copyFile(path, dst); err != nil {
			return err
		}
		copied++
		return nil
	})
	return copied, err
}

// copyFile 复制单个文件（保留权限位）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// dataDirSourceLabel 把来源转成界面上的一句话。
func dataDirSourceLabel(src string) string {
	if runtime.GOOS == "windows" {
		return src
	}
	return src
}
