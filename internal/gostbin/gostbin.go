// Package gostbin 定位、下载（可选嵌入）gost 二进制。
//
// 查找优先级：用户指定路径 > 嵌入（embedded 构建）> 数据目录 > PATH > 下载。
package gostbin

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"mygost/internal/proc"
)

// DefaultVersion 是下载使用的默认版本（不含 v 前缀）。
const DefaultVersion = "3.3.1"

// DefaultMirror 是下载地址模板。
const DefaultMirror = "https://github.com/go-gost/gost/releases/download/v{version}/gost_{version}_{os}_{arch}.{ext}"

// ErrNotFound 表示未能找到 gost 二进制。
var ErrNotFound = errors.New("未找到 gost，请下载或手动指定路径")

// Progress 上报下载进度，total 为 -1 表示未知。
type Progress func(done, total int64)

// Locator 管理二进制的查找与下载，dir 是下载存放目录。
type Locator struct {
	mu sync.Mutex
	// lastErr 记录最近一次解压内置副本的失败原因（供界面显示）
	lastErr error

	Dir string
}

// binaryName 按平台返回二进制名。
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "gost.exe"
	}
	return "gost"
}

// Find 按优先级查找，source 描述来源（manual/downloaded/embedded/path）。
//
//	用户指定路径 > 已下载副本 > 构建时内置 > 系统 PATH
//
// 已下载副本排在内置之前：用户主动点「下载/更新」应当生效，
// 否则内置版本会永远盖住它。
func (l *Locator) Find(manualPath string) (path, source string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if manualPath != "" {
		if st, e := os.Stat(manualPath); e == nil && !st.IsDir() && st.Size() > 0 {
			return manualPath, "manual", nil
		}
	}
	if p := l.downloadedPath(); p != "" {
		return p, "downloaded", nil
	}
	if p, ok := l.ensureEmbedded(); ok {
		return p, "embedded", nil
	}
	if p, e := exec.LookPath("gost"); e == nil {
		return p, "path", nil
	}
	// 带着内置副本的失败原因返回：Windows 上多半是安全软件拦截写盘
	if le := l.lastErr; le != nil {
		return "", "", fmt.Errorf("%w（内置副本不可用：%v）", ErrNotFound, le)
	}
	return "", "", ErrNotFound
}

// downloadedPath 返回已下载副本的路径（不存在时为空）。
// 兼容旧布局：早期版本把下载的二进制直接放在 Dir/gost。
func (l *Locator) downloadedPath() string {
	p := filepath.Join(l.Dir, binaryName())
	if st, e := os.Stat(p); e == nil && !st.IsDir() && st.Size() > 0 {
		return p
	}
	return ""
}

// DownloadURL 按模板展开下载地址。
func DownloadURL(mirror string) string {
	if strings.TrimSpace(mirror) == "" {
		mirror = DefaultMirror
	}
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	r := strings.NewReplacer(
		"{version}", DefaultVersion,
		"{os}", runtime.GOOS,
		"{arch}", runtime.GOARCH,
		"{ext}", ext,
		"{bin}", binaryName(),
	)
	return r.Replace(mirror)
}

// Download 下载并安装 gost 到 l.Dir，完成后返回其路径。
// 会同时下载 checksums.txt 做 sha256 校验（获取失败不阻塞，仅跳过校验）。
func (l *Locator) Download(ctx context.Context, mirror string, onProgress Progress) (string, error) {
	url := DownloadURL(mirror)
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return "", err
	}
	data, err := httpGet(ctx, url, onProgress)
	if err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])

	// 校验（archive 内文件的 sha256）
	archiveName := "gost_" + DefaultVersion + "_" + runtime.GOOS + "_" + runtime.GOARCH + "."
	if runtime.GOOS == "windows" {
		archiveName += "zip"
	} else {
		archiveName += "tar.gz"
	}
	if sums, err := httpGet(ctx, strings.TrimSuffix(url, "/"+filepath.Base(url))+"/checksums.txt", nil); err == nil {
		if !checksumOK(string(sums), archiveName, got) {
			return "", fmt.Errorf("sha256 校验失败，已放弃安装")
		}
	}

	entry, err := extractBinary(data, runtime.GOOS == "windows")
	if err != nil {
		return "", err
	}
	final := filepath.Join(l.Dir, binaryName())
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, entry, 0o755); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		os.Chmod(tmp, 0o755)
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return final, nil
}

// Version 返回 `gost -V` 的首行输出。
func Version(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 隐藏窗口：Windows 上每次版本检测都会闪一个黑框
	out, err := proc.HideWindow(exec.CommandContext(ctx, path, "-V")).Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line
}

func checksumOK(sums, name, want string) bool {
	for _, line := range strings.Split(sums, "\n") {
		if h, f, ok := strings.Cut(strings.TrimSpace(line), "  "); ok && f == name {
			return h == want
		}
	}
	return false
}

func httpGet(ctx context.Context, rawURL string, onProgress Progress) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mygost/"+DefaultVersion)

	// 代理感知：Windows 用户常靠系统代理上网，但 Go 的默认传输
	// 在 Windows 上不读系统设置，只认环境变量。这里显式接上系统代理，
	// 否则国内用户直连 GitHub 下载几乎必败。
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if p := systemProxy(); p != "" {
		if pu, pe := url.Parse(p); pe == nil {
			transport.Proxy = func(*http.Request) (*url.URL, error) { return pu, nil }
		}
	}
	client := &http.Client{Timeout: 10 * time.Minute, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 0 && onProgress != nil {
		pr := &progressReader{r: resp.Body, total: resp.ContentLength, on: onProgress}
		return io.ReadAll(pr)
	}
	data, err := io.ReadAll(resp.Body)
	if err == nil && onProgress != nil {
		onProgress(int64(len(data)), int64(len(data)))
	}
	return data, err
}

type progressReader struct {
	r     io.Reader
	total int64
	done  int64
	last  time.Time
	on    Progress
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.on != nil && time.Since(p.last) > 100*time.Millisecond {
		p.last = time.Now()
		p.on(p.done, p.total)
	}
	return n, err
}

// extractBinary 从 tar.gz 或 zip 中取出 gost 二进制内容。
func extractBinary(data []byte, isZip bool) ([]byte, error) {
	if isZip {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			base := filepath.Base(f.Name)
			if base == "gost.exe" || (f.Name == "gost.exe" || strings.HasSuffix(base, ".exe")) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("压缩包中未找到 gost.exe")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == "gost" && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("压缩包中未找到 gost")
}

// ExplainError 把下载错误翻译成用户能行动的提示。
//
// Windows 上最常见的是"直连 GitHub 不可达"与"系统代理没被读到的焦虑"，
// 这两类错误原文（dial tcp ... i/o timeout）对用户毫无帮助。
func ExplainError(err error, mirror string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "i/o timeout"), strings.Contains(lower, "deadline exceeded"):
		return msg + "\n\n网络超时。若身处境内，GitHub 通常无法直连：" +
			"请在设置里填写镜像地址（支持 {version} {os} {arch} {ext} 变量），" +
			"或先开启系统代理后重试。"
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "server misbehaving"):
		return msg + "\n\n域名解析失败。检查网络与 DNS，或改用镜像地址。"
	case strings.Contains(lower, "connection refused"), strings.Contains(lower, "connection reset"):
		return msg + "\n\n连接被拒绝/重置。若正在用代理，确认代理端口可用；" +
			"也可以改用镜像地址。"
	case strings.Contains(lower, "sha256"):
		return msg + "\n\n下载内容校验不通过（可能被中间代理篡改或镜像不完整），" +
			"请更换镜像重试。"
	case strings.Contains(lower, "http 404"), strings.Contains(lower, "http 403"):
		return msg + "\n\n镜像地址可能不对。默认镜像为 " + DefaultMirror
	}
	if mirror == "" {
		return msg
	}
	return msg
}
