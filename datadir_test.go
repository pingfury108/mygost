package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 指针读写：写入后能读回，清除后回到默认。
func TestDataDirPointer(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base) // Windows 上 UserConfigDir 读它
	t.Setenv("HOME", base)    // macOS/Linux
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, ".config"))
	t.Setenv("MYGOST_DATA_DIR", "")

	// 清掉可能残留的指针
	_ = clearPointer()

	if got := readPointer(); got != "" {
		t.Fatalf("初始不应有指针，got %q", got)
	}

	custom := filepath.Join(base, "custom-data")
	if err := writePointer(custom); err != nil {
		t.Fatal(err)
	}
	if got := readPointer(); got != custom {
		t.Errorf("读回 %q, want %q", got, custom)
	}

	// 解析时应优先用指针
	dir, src, warn := resolveDataDir()
	if dir != custom {
		t.Errorf("resolveDataDir = %q, want %q", dir, custom)
	}
	if warn != "" {
		t.Errorf("不应有回退警告: %s", warn)
	}
	if src == "" {
		t.Error("应说明来源")
	}

	// 清除后回到默认
	if err := clearPointer(); err != nil {
		t.Fatal(err)
	}
	if got := readPointer(); got != "" {
		t.Errorf("清除后不应有指针，got %q", got)
	}
}

// 指针内容无效（相对路径/空白）时视为没有指针。
func TestDataDirPointerInvalid(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("HOME", base)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, ".config"))
	t.Setenv("MYGOST_DATA_DIR", "")

	if err := os.MkdirAll(defaultDataDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"", "   ", "relative/path\n"} {
		if err := os.WriteFile(pointerPath(), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := readPointer(); got != "" {
			t.Errorf("内容 %q 应被视为无效，got %q", content, got)
		}
	}
}

// 自定义目录不可用时回退默认目录，并带回原因。
func TestDataDirFallbackWhenUnavailable(t *testing.T) {
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	t.Setenv("HOME", base)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, ".config"))
	t.Setenv("MYGOST_DATA_DIR", "")

	// 用一个"文件"充当目录，必然不可用
	fake := filepath.Join(base, "not-a-dir")
	os.WriteFile(fake, []byte("x"), 0o644)
	if err := writePointer(fake); err != nil {
		t.Fatal(err)
	}

	dir, _, warn := resolveDataDir()
	if dir != defaultDataDir() {
		t.Errorf("应回退到默认目录 %q，got %q", defaultDataDir(), dir)
	}
	if warn == "" {
		t.Error("回退时应给出原因")
	}
}

// 目录校验：拒绝当前目录、子目录、相对路径。
func TestValidateDataDirTarget(t *testing.T) {
	base := t.TempDir()
	cur := filepath.Join(base, "cur")
	os.MkdirAll(cur, 0o755)

	if err := validateDataDirTarget(cur, cur); err == nil {
		t.Error("不应允许选择当前目录")
	}
	if err := validateDataDirTarget(cur, filepath.Join(cur, "sub")); err == nil {
		t.Error("不应允许选择当前目录的子目录")
	}
	if err := validateDataDirTarget(cur, "relative"); err == nil {
		t.Error("不应允许相对路径")
	}
	// 同级目录可以
	if err := validateDataDirTarget(cur, filepath.Join(base, "other")); err != nil {
		t.Errorf("同级目录应被接受: %v", err)
	}
}

// 迁移：复制内容但跳过 state/（进程状态），不删除原目录。
func TestMigrateDataDir(t *testing.T) {
	base := t.TempDir()
	from := filepath.Join(base, "from")
	to := filepath.Join(base, "to")
	os.MkdirAll(filepath.Join(from, "profiles"), 0o755)
	os.MkdirAll(filepath.Join(from, "state"), 0o755)
	os.WriteFile(filepath.Join(from, "settings.json"), []byte(`{"a":1}`), 0o644)
	os.WriteFile(filepath.Join(from, "profiles", "p1.json"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(from, "state", "gost.pid"), []byte(`{"pid":1}`), 0o644)

	n, err := migrateDataDir(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("应复制 2 个文件，实际 %d", n)
	}
	if _, err := os.Stat(filepath.Join(to, "settings.json")); err != nil {
		t.Error("settings.json 未复制")
	}
	if _, err := os.Stat(filepath.Join(to, "profiles", "p1.json")); err != nil {
		t.Error("profiles 未复制")
	}
	if _, err := os.Stat(filepath.Join(to, "state")); !os.IsNotExist(err) {
		t.Error("state/ 不应被复制")
	}
	// 原目录保留
	if _, err := os.Stat(filepath.Join(from, "settings.json")); err != nil {
		t.Error("原目录不应被删除")
	}
}

// 迁移不应覆盖目标已存在的文件。
func TestMigrateKeepsExisting(t *testing.T) {
	base := t.TempDir()
	from := filepath.Join(base, "from")
	to := filepath.Join(base, "to")
	os.MkdirAll(from, 0o755)
	os.MkdirAll(to, 0o755)
	os.WriteFile(filepath.Join(from, "settings.json"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(to, "settings.json"), []byte("new"), 0o644)

	if _, err := migrateDataDir(from, to); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(to, "settings.json"))
	if string(b) != "new" {
		t.Errorf("目标已有文件被覆盖: %q", b)
	}
}

// 环境变量清理：去引号与末尾分隔符。
func TestCleanDirEnv(t *testing.T) {
	cases := map[string]string{
		`  "D:\mygost"  `: "D:\\mygost",
		`'D:\mygost'`:     "D:\\mygost",
		`D:\mygost\`:      "D:\\mygost\\",
		`  `:              "",
	}
	for in, want := range cases {
		if got := cleanDirEnv(in); got != want {
			t.Errorf("cleanDirEnv(%q) = %q, want %q", in, got, want)
		}
	}
}

// dev 版与正式版的自定义指针互不影响。
func TestPointerNamePerVariant(t *testing.T) {
	name := pointerName()
	if !strings.HasPrefix(name, "datadir") {
		t.Errorf("指针文件名异常: %q", name)
	}
	if runtime.GOOS != "" && dataDirName() == "mygost-dev" && !strings.HasSuffix(name, "-dev") {
		t.Errorf("dev 版指针应带 -dev 后缀，got %q", name)
	}
}
