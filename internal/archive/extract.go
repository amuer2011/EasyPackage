package archive

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// JPGReader also accepts ZIP files, so password ZIPs can be restored without an
// external extractor. The JPEG footer is identical to earlier EasyPackage releases.
func JPGReader(input *os.File) (*zip.Reader, error) {
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	header := make([]byte, 4)
	_, err = input.ReadAt(header, 0)
	if err != nil {
		return nil, err
	}
	if string(header[:2]) == "PK" {
		return zip.NewReader(input, info.Size())
	}
	end := info.Size() - int64(len(magic)+8)
	if end <= 0 {
		return nil, errors.New("该文件不包含 EasyPackage 附件")
	}
	footer := make([]byte, len(magic)+8)
	if _, err = input.ReadAt(footer, end); err != nil {
		return nil, err
	}
	start := binary.BigEndian.Uint64(footer[:8])
	if string(footer[8:]) != magic || start < 3 || start >= uint64(end) {
		return nil, errors.New("未找到有效的 ZIP 附件")
	}
	section := io.NewSectionReader(input, int64(start), end-int64(start))
	return zip.NewReader(section, section.Size())
}

func safeEntryName(name string) (string, error) {
	clean := strings.TrimSuffix(name, "/")
	if !fs.ValidPath(clean) || strings.ContainsAny(clean, "\\:\x00") {
		return "", fmt.Errorf("ZIP 含不安全路径：%s", name)
	}
	for _, part := range strings.Split(clean, "/") {
		if strings.TrimRight(part, ". ") != part || strings.ContainsAny(part, "<>\"|?*") {
			return "", fmt.Errorf("ZIP 文件名不兼容：%s", name)
		}
		for _, c := range part {
			if c < 32 {
				return "", fmt.Errorf("ZIP 文件名含控制字符")
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
			return "", fmt.Errorf("ZIP 含系统保留文件名：%s", name)
		}
	}
	return filepath.FromSlash(clean), nil
}

// Extract restores into a NEW child directory. Existing directories are never
// merged or replaced. Temporary extraction is private and removed on all failures.
func Extract(ctx context.Context, inputPath, output, password string, report func(int64, string)) error {
	if strings.TrimSpace(output) == "" {
		return errors.New("请选择还原位置")
	}
	absolute, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	parent, err := canonical(filepath.Dir(absolute))
	if err != nil {
		return err
	}
	out := filepath.Join(parent, filepath.Base(absolute))
	if _, err = os.Lstat(out); err == nil {
		return errors.New("还原文件夹已存在，请换一个新文件夹名称")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()
	reader, err := JPGReader(input)
	if err != nil {
		return err
	}
	// Validate all paths and entry kinds before writing any content.
	seen := map[string]bool{}
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, err := safeEntryName(entry.Name)
		if err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("ZIP 含重复路径：%s", entry.Name)
		}
		seen[name] = true
		if kind := entry.Mode().Type(); kind != 0 && kind != fs.ModeDir {
			return errors.New("不允许解压符号链接或特殊文件")
		}
		if entry.Flags&1 != 0 && password == "" {
			return errors.New("此压缩包已加密，请输入密码")
		}
	}
	stage, err := os.MkdirTemp(parent, ".easypackage-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage) // stage is exclusively created by this task under parent.
	root, err := os.OpenRoot(stage)
	if err != nil {
		return err
	}
	defer root.Close()
	var done int64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, _ := safeEntryName(entry.Name)
		if entry.FileInfo().IsDir() {
			if err = root.MkdirAll(name, 0755); err != nil {
				return err
			}
			if err = copyEntry(ctx, entry, password, io.Discard); err != nil {
				return err
			}
			continue
		}
		if err = root.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return err
		}
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		err = copyEntry(ctx, entry, password, reportWriter{file, &done, entry.Name, report})
		if err == nil {
			err = file.Chmod(entry.Mode().Perm() | 0600)
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if !entry.Modified.IsZero() {
			if err = os.Chtimes(filepath.Join(stage, name), entry.Modified, entry.Modified); err != nil {
				return err
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = root.Close(); err != nil {
		return err
	}
	// Reserve the target exclusively; never replace somebody else's directory.
	if err = os.Mkdir(out, 0700); err != nil {
		return err
	}
	reserved, err := os.Lstat(out)
	if err != nil {
		os.Remove(out)
		return err
	}
	// Moving children avoids platform-specific directory replacement semantics.
	// On failure, remove only the directory we just reserved, never preexisting data.
	published := false
	defer func() {
		if !published {
			if current, e := os.Lstat(out); e == nil && os.SameFile(current, reserved) {
				os.RemoveAll(out)
			}
		}
	}()
	children, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = os.Rename(filepath.Join(stage, child.Name()), filepath.Join(out, child.Name())); err != nil {
			return err
		}
	}
	if err = os.Chmod(out, 0755); err != nil {
		return err
	}
	published = true
	return nil
}
