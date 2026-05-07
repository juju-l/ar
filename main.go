package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"golang.org/x/oauth2/google"
)

// 适配GCP AR的Scope
const arScope = "https://www.googleapis.com/auth/cloud-platform"

// gcpARAuth 使用 golang.org/x/oauth2/google 获取ADC/Workload Identity凭证
func gcpARAuth(ctx context.Context) (authn.Authenticator, error) {
	// 自动优先级：
	// 1. K8s Workload Identity 元数据
	// 2. 本地ADC凭证
	// 3. GCE/GKE 元数据服务
	ts, err := google.DefaultTokenSource(ctx, arScope)
	if err != nil {
		return nil, fmt.Errorf("get default token source: %w", err)
	}

	// 转为 go-containerregistry 认证器
	return authn.FromMethod(authn.MethodFunc(func() (*authn.AuthConfig, error) {
		tok, err := ts.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetch token: %w", err)
		}
		// GCP AR 固定用户名 _token，密码为access_token
		return &authn.AuthConfig{
			Username: "_token",
			Password: tok.AccessToken,
		}, nil
	})), nil
}

// BuildScratchFromDir 构建scratch镜像并推送到GCP AR
// arRef: 区域-docker.pkg.dev/project/repo/img:tag
// localDir: 本地目录，等价 COPY . /workspace
func BuildScratchFromDir(ctx context.Context, arRef, localDir string) error {
	// 每次构建独立临时目录，并发安全隔离
	tmpWorkDir, err := os.MkdirTemp("", "gcp-build-*")
	if err != nil {
		return fmt.Errorf("mk temp dir: %w", err)
	}
	defer os.RemoveAll(tmpWorkDir)

	tarPath := filepath.Join(tmpWorkDir, "layer.tar.gz")

	if err := packDirToTarGz(localDir, tarPath, "/workspace"); err != nil {
		return fmt.Errorf("pack dir: %w", err)
	}

	layer, err := tarball.LayerFromFile(tarPath)
	if err != nil {
		return fmt.Errorf("load layer: %w", err)
	}

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return fmt.Errorf("append layer: %w", err)
	}

	ref, err := name.ParseReference(arRef)
	if err != nil {
		return fmt.Errorf("parse ar ref: %w", err)
	}

	auth, err := gcpARAuth(ctx)
	if err != nil {
		return fmt.Errorf("get gcp auth: %w", err)
	}

	if err := mutate.Push(ctx, img, ref, auth); err != nil {
		return fmt.Errorf("push to ar failed: %w", err)
	}

	fmt.Printf("✅ 成功推送到GCP AR: %s\n", arRef)
	return nil
}

// packDirToTarGz 本地目录打包为tar.gz，映射到容器 /workspace
func packDirToTarGz(srcDir, tarGzPath, containerPrefix string) error {
	fw, err := os.Create(tarGzPath)
	if err != nil {
		return err
	}
	defer fw.Close()

	gw := gzip.NewWriter(fw)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		containerPath := filepath.Join(containerPrefix, relPath)
		containerPath = strings.ReplaceAll(containerPath, "\\", "/")

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = containerPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		}
		return nil
	})
}

func main() {
	ctx := context.Background()
	// 替换成你的AR地址
	arImageRef := "cn-beijing-docker.pkg.dev/你的项目ID/你的AR仓库/scratch-app:v1"
	localDir := "./"

	if err := BuildScratchFromDir(ctx, arImageRef, localDir); err != nil {
		fmt.Printf("❌ 构建推送失败: %v\n", err)
		os.Exit(1)
	}
}
