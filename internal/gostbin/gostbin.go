// Package gostbin 定位、下载（可选嵌入）gost 二进制。
//
// 查找优先级：用户指定路径 > 嵌入（embedded 构建）> 数据目录 > PATH > 下载。
package gostbin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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
	if manualPath != "" {
		if st, e := os.Stat(manualPath); e == nil && !st.IsDir() {
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
	return "", "", ErrNotFound
}

// downloadedPath 返回已下载副本的路径（不存在时为空）。
// 兼容旧布局：早期版本把下载的二进制直接放在 Dir/gost。
func (l *Locator) downloadedPath() string {
	p := filepath.Join(l.Dir, binaryName())
	if st, e := os.Stat(p); e == nil && !st.IsDir() {
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
	out, err := exec.CommandContext(ctx, path, "-V").Output()
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

func httpGet(ctx context.Context, url string, onProgress Progress) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
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
		zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
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
	gz, err := gzip.NewReader(strings.NewReader(string(data)))
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
