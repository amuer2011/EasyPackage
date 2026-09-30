// Package archive implements project ZIP archives and reversible JPEG attachments.
package archive

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const magic = "EASYPACKAGE-JPG1" // Compatible with the previous Java edition.

type Options struct {
	Source      string `json:"source"`
	Output      string `json:"output"`
	FilterClass bool   `json:"filterClass"`
	MaxBytes    int64  `json:"maxBytes"`
	Cover       string `json:"cover"`
	Overwrite   bool   `json:"overwrite"`
	Password    string `json:"password"`
}
type Item struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Directory bool   `json:"directory"`
	info      fs.FileInfo
}
type Plan struct {
	Items        []Item `json:"items"`
	Bytes        int64  `json:"bytes"`
	Files        int    `json:"files"`
	Filtered     int    `json:"filtered"`
	Links        int    `json:"links"`
	ExcludedDirs int    `json:"excludedDirs"`
	root         string
}

func canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
func destination(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("请选择输出文件")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := canonical(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("保存目录不可用：%w", err)
	}
	stat, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	if !stat.IsDir() {
		return "", errors.New("保存位置必须是文件夹")
	}
	out := filepath.Join(parent, filepath.Base(abs))
	if info, err := os.Lstat(out); err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("输出位置不是普通文件，或是符号链接")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return out, nil
}
func same(a, b string) bool {
	ai, e := os.Stat(a)
	if e != nil {
		return false
	}
	bi, e := os.Stat(b)
	return e == nil && os.SameFile(ai, bi)
}

func Scan(ctx context.Context, o Options) (Plan, error) {
	p := Plan{Items: []Item{}}
	if strings.TrimSpace(o.Source) == "" {
		return p, errors.New("请选择项目文件夹")
	}
	if o.MaxBytes < 0 {
		return p, errors.New("文件大小阈值不能为负数")
	}
	root, err := canonical(o.Source)
	if err != nil {
		return p, err
	}
	st, err := os.Stat(root)
	if err != nil {
		return p, err
	}
	if !st.IsDir() {
		return p, errors.New("项目路径必须是文件夹")
	}
	p.root = root
	out, err := destination(o.Output)
	if err != nil {
		return p, err
	}
	outputInfo, _ := os.Stat(out)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if d.IsDir() && excludedDirectory(d.Name()) {
			p.ExcludedDirs++
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			p.Links++
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if info.IsDir() {
			p.Items = append(p.Items, Item{Name: name + "/", Directory: true, info: info})
			return nil
		}
		if path == out || (outputInfo != nil && os.SameFile(info, outputInfo)) || (o.FilterClass && strings.EqualFold(filepath.Ext(name), ".class")) || (o.MaxBytes > 0 && info.Size() > o.MaxBytes) {
			p.Filtered++
			return nil
		}
		if p.Bytes > math.MaxInt64-info.Size() {
			return errors.New("项目总大小超过支持范围")
		}
		p.Bytes += info.Size()
		p.Files++
		p.Items = append(p.Items, Item{Name: name, Size: info.Size(), info: info})
		return nil
	})
	return p, err
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

type reportWriter struct {
	w       io.Writer
	written *int64
	name    string
	report  func(int64, string)
}

func (w reportWriter) Write(b []byte) (int, error) {
	n, e := w.w.Write(b)
	*w.written += int64(n)
	if w.report != nil {
		w.report(*w.written, w.name)
	}
	return n, e
}

// writeOutput stages data beside the destination; failure never publishes a partial archive.
func writeOutput(ctx context.Context, path string, overwrite bool, fn func(*os.File) error) (err error) {
	out, err := destination(path)
	if err != nil {
		return err
	}
	if !overwrite {
		if _, e := os.Lstat(out); e == nil {
			return errors.New("输出文件已存在，请确认覆盖")
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	f, err := os.CreateTemp(filepath.Dir(out), ".easypackage-*.tmp")
	if err != nil {
		return err
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()
	if err = fn(f); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if !overwrite {
		// Reserve the name exclusively, including on FAT/exFAT and network drives.
		reservation, e := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return fmt.Errorf("无法创建输出文件（目标可能已存在）：%w", e)
		}
		reservedInfo, e := reservation.Stat()
		closeErr := reservation.Close()
		defer func() {
			if err != nil && reservedInfo != nil {
				if info, e := os.Lstat(out); e == nil && os.SameFile(info, reservedInfo) {
					os.Remove(out)
				}
			}
		}()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return os.Rename(f.Name(), out)
}

func Pack(ctx context.Context, o Options, p Plan, report func(int64, string)) error {
	if p.root == "" {
		return errors.New("请先扫描项目")
	}
	if o.Cover != "" && same(o.Cover, o.Output) {
		return errors.New("输出文件不能覆盖 JPG 封面")
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return err
	}
	defer root.Close()
	return writeOutput(ctx, o.Output, o.Overwrite, func(f *os.File) error {
		var prefix int64
		if o.Cover != "" {
			image, err := os.Open(o.Cover)
			if err != nil {
				return err
			}
			defer image.Close()
			if _, err = jpeg.DecodeConfig(contextReader{ctx, image}); err != nil {
				return fmt.Errorf("无效的 JPG 图片：%w", err)
			}
			if _, err = image.Seek(0, io.SeekStart); err != nil {
				return err
			}
			prefix, err = io.Copy(f, contextReader{ctx, image})
			if err != nil {
				return err
			}
		}
		zw := zip.NewWriter(f)
		var written int64
		for _, item := range p.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := filepath.FromSlash(strings.TrimSuffix(item.Name, "/"))
			info, err := root.Lstat(name)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || !os.SameFile(item.info, info) {
				return fmt.Errorf("文件在扫描后已替换，请重试：%s", item.Name)
			}
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = item.Name
			if !item.Directory && o.Password != "" {
				if err := writeEncryptedEntry(ctx, zw, header, o.Password, filepath.Dir(o.Output), func(dest io.Writer) error {
					return copyItem(ctx, root, item, dest, &written, report)
				}); err != nil {
					return err
				}
				continue
			}
			if !item.Directory {
				header.Method = zip.Deflate
			}
			dest, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}
			if item.Directory {
				continue
			}
			if err := copyItem(ctx, root, item, dest, &written, report); err != nil {
				return err
			}
		}
		if err := zw.Close(); err != nil {
			return err
		}
		if o.Cover != "" {
			if err := binary.Write(f, binary.BigEndian, uint64(prefix)); err != nil {
				return err
			}
			_, err := io.WriteString(f, magic)
			return err
		}
		return nil
	})
}

func excludedDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".idea", ".codegraph", ".vscode", ".vs", ".svn", ".hg", ".fleet":
		return true
	default:
		return false
	}
}

func copyItem(ctx context.Context, root *os.Root, item Item, dest io.Writer, written *int64, report func(int64, string)) error {
	input, err := root.Open(filepath.FromSlash(item.Name))
	if err != nil {
		return err
	}
	defer input.Close()
	current, err := input.Stat()
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(item.info, current) {
		return fmt.Errorf("文件在打包时已替换：%s", item.Name)
	}
	n, err := io.Copy(reportWriter{dest, written, item.Name, report}, contextReader{ctx, io.LimitReader(input, item.Size+1)})
	if err != nil {
		return err
	}
	after, err := input.Stat()
	if err != nil {
		return err
	}
	if n != item.Size || after.Size() != item.Size || !after.ModTime().Equal(item.info.ModTime()) {
		return fmt.Errorf("文件在扫描后发生变化，请重试：%s", item.Name)
	}
	return nil
}

func Restore(ctx context.Context, image, output string, overwrite bool) error {
	if same(image, output) {
		return errors.New("还原路径不能与 JPG 文件相同")
	}
	input, err := os.Open(image)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	end := info.Size() - int64(len(magic)+8)
	if end <= 0 {
		return errors.New("该图片不包含 EasyPackage 附件")
	}
	footer := make([]byte, len(magic)+8)
	if _, err = input.ReadAt(footer, end); err != nil {
		return err
	}
	start := binary.BigEndian.Uint64(footer[:8])
	if string(footer[8:]) != magic || start < 3 || start >= uint64(end) {
		return errors.New("未找到有效的 ZIP 附件")
	}
	section := io.NewSectionReader(input, int64(start), end-int64(start))
	reader, err := zip.NewReader(section, section.Size())
	if err != nil {
		return fmt.Errorf("ZIP 附件损坏：%w", err)
	}
	// Reading each entry to EOF checks ZIP's CRC before replacing an existing output.
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, err := entry.Open()
		if err != nil {
			return err
		}
		_, err = io.Copy(io.Discard, contextReader{ctx, r})
		closeErr := r.Close()
		if err != nil {
			return fmt.Errorf("ZIP 完整性校验失败：%w", err)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return writeOutput(ctx, output, overwrite, func(f *os.File) error { _, err := io.Copy(f, contextReader{ctx, section}); return err })
}
