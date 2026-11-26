// 本文件限制真实浏览器只服务本项目的两处独立 Creator 构建位置。
package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// creatorBrowserBuildPath 核对实际解析路径，拒绝浏览器测试连接到主试玩目录。
func creatorBrowserBuildPath(repo, raw string) (string, error) {
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return "", err
	}
	build, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	build, err = filepath.EvalSymlinks(build)
	if err != nil {
		return "", err
	}
	roots := []string{filepath.Join(repo, ".work")}
	if local := os.Getenv("LOCALAPPDATA"); runtime.GOOS == "windows" && local != "" {
		if directory, e := filepath.EvalSymlinks(local); e == nil {
			digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(repo))))
			parent := filepath.Join(directory, "minimir-creator-work", hex.EncodeToString(digest[:8]))
			if resolved, e := filepath.EvalSymlinks(parent); e == nil && strings.EqualFold(parent, resolved) {
				roots = append(roots, parent)
			}
		}
	}
	for _, root := range roots {
		relative, e := filepath.Rel(root, build)
		if e != nil || !filepath.IsLocal(relative) || relative == "." {
			continue
		}
		index, e := os.Stat(filepath.Join(build, "index.html"))
		if e != nil || !index.Mode().IsRegular() {
			return "", errors.New("验收目录缺少 Creator 网页入口")
		}
		return build, nil
	}
	return "", errors.New("浏览器只能使用本项目的独立 Creator 构建")
}
