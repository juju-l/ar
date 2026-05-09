package main

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// path2TarGzWalk
// rootPath: 源根目录
// innerPath: tar包内顶层目录
// tw: 外部初始化好的 tar.Writer
// include/exclude: 支持 *.log、conf/a-*-9.txt、var/** 规则
func path2TarGzWalk(rootPath, innerPath string, tw *tar.Writer, include, exclude []string) error {
	return filepath.Walk(rootPath, func(filePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// 1. 计算相对路径并统一分隔符
		rel, err := filepath.Rel(rootPath, filePath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		// 2. 内部即时编译并匹配 exclude
		matchExclude := false
		for _, pat := range exclude {
			regStr := regexp.QuoteMeta(pat)
			regStr = strings.ReplaceAll(regStr, `\*\*`, `.*`)
			regStr = strings.ReplaceAll(regStr, `\*`, `[^/]*`)
			regStr = "^" + regStr + "$"
			reg := regexp.MustCompile(regStr)
			if reg.MatchString(rel) {
				matchExclude = true
				break
			}
		}
		if matchExclude {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 3. 内部即时编译并匹配 include
		matchInclude := true
		if len(include) > 0 {
			matchInclude = false
			for _, pat := range include {
				regStr := regexp.QuoteMeta(pat)
				regStr = strings.ReplaceAll(regStr, `\*\*`, `.*`)
				regStr = strings.ReplaceAll(regStr, `\*`, `[^/]*`)
				regStr = "^" + regStr + "$"
				reg := regexp.MustCompile(regStr)
				if reg.MatchString(rel) {
					matchInclude = true
					break
				}
			}
		}
		if !matchInclude {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 4. 只保留普通文件、软链接，跳过其他特殊文件
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		// 5. 构造包内归档路径
		arcPath := filepath.Join(innerPath, rel)
		arcPath = filepath.ToSlash(arcPath)

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = arcPath

		// 6. 处理软链接，转为包内相对路径
		if info.Mode()&os.ModeSymlink != 0 {
			linkDest, err := os.Readlink(filePath)
			if err != nil {
				return err
			}

			absFile, err := filepath.Abs(filePath)
			if err != nil {
				return err
			}
			fileDir := filepath.Dir(absFile)

			relLink, err := filepath.Rel(fileDir, linkDest)
			if err != nil {
				relLink = linkDest
			}
			hdr.Linkname = filepath.ToSlash(relLink)

			return tw.WriteHeader(hdr)
		}

		// 目录只写header
		if info.IsDir() {
			return tw.WriteHeader(hdr)
		}

		// 普通文件写入内容
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
}
