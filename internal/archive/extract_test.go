package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExcludedDirectoriesAtEveryDepth(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "project")
	for _, name := range []string{".git", ".IDEA", ".codegraph", ".vscode", ".vs", ".svn", ".hg", ".fleet"} {
		put(t, filepath.Join(source, "nested", name, "deeper", "secret"), []byte("omit"))
	}
	for _, name := range []string{".github/workflow.yml", ".env", "node_modules/lib/index.js", "target/resources/data.txt"} {
		put(t, filepath.Join(source, filepath.FromSlash(name)), []byte("keep"))
	}
	o := Options{Source: source, Output: filepath.Join(dir, "out.zip")}
	p, e := Scan(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if p.Files != 4 || p.ExcludedDirs != 8 {
		t.Fatalf("unexpected exclusion: %+v", p)
	}
	if e = Pack(context.Background(), o, p, nil); e != nil {
		t.Fatal(e)
	}
	for name := range entries(t, o.Output) {
		if bytes.Contains([]byte(name), []byte("secret")) {
			t.Fatal("excluded child leaked")
		}
	}
}
func TestEncryptedZIPAndJPGExtract(t *testing.T) {
	for _, jpg := range []bool{false, true} {
		t.Run(map[bool]string{false: "ZIP", true: "JPG"}[jpg], func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			content := []byte("中文密码测试\nhello world!")
			put(t, filepath.Join(source, "子目录", "你好.txt"), content)
			put(t, filepath.Join(source, "empty.txt"), nil)
			os.Mkdir(filepath.Join(source, "空目录"), 0755)
			o := Options{Source: source, Output: filepath.Join(dir, "encrypted.zip"), Password: "测试-password!"}
			if jpg {
				o.Output = filepath.Join(dir, "encrypted.jpg")
				o.Cover = filepath.Join(dir, "cover.jpg")
				jpegFile(t, o.Cover)
			}
			p, e := Scan(ctx, o)
			if e != nil {
				t.Fatal(e)
			}
			if e = Pack(ctx, o, p, nil); e != nil {
				t.Fatal(e)
			}
			out := filepath.Join(dir, "restored")
			for _, password := range []string{"", "wrong"} {
				if e = Extract(ctx, o.Output, out, password, nil); e == nil {
					t.Fatal("missing/wrong password accepted")
				}
				if _, e = os.Stat(out); !errors.Is(e, os.ErrNotExist) {
					t.Fatal("failure left output")
				}
			}
			if e = Extract(ctx, o.Output, out, o.Password, nil); e != nil {
				t.Fatal(e)
			}
			actual, e := os.ReadFile(filepath.Join(out, "子目录", "你好.txt"))
			if e != nil || !bytes.Equal(actual, content) {
				t.Fatalf("roundtrip: %q %v", actual, e)
			}
			if st, e := os.Stat(filepath.Join(out, "空目录")); e != nil || !st.IsDir() {
				t.Fatal("directory missing")
			}
			if e = Extract(ctx, o.Output, out, o.Password, nil); e == nil {
				t.Fatal("existing output overwritten")
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if e = Extract(cancelled, o.Output, filepath.Join(dir, "cancelled"), o.Password, nil); !errors.Is(e, context.Canceled) {
				t.Fatalf("cancellation ignored: %v", e)
			}
			pending, _ := filepath.Glob(filepath.Join(dir, ".easypackage-*"))
			if len(pending) != 0 {
				t.Fatalf("leftovers: %v", pending)
			}
		})
	}
}
func TestEncryptedAuthenticationFailure(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	put(t, filepath.Join(source, "data"), []byte("authenticated content"))
	o := Options{Source: source, Output: filepath.Join(dir, "encrypted.zip"), Password: "secret"}
	p, _ := Scan(context.Background(), o)
	if e := Pack(context.Background(), o, p, nil); e != nil {
		t.Fatal(e)
	}
	r, e := zip.OpenReader(o.Output)
	if e != nil {
		t.Fatal(e)
	}
	offset, e := r.File[0].DataOffset()
	if e != nil {
		t.Fatal(e)
	}
	r.Close()
	b, _ := os.ReadFile(o.Output)
	b[offset+18] ^= 1
	put(t, o.Output, b)
	out := filepath.Join(dir, "result")
	if e = Extract(context.Background(), o.Output, out, o.Password, nil); e == nil {
		t.Fatal("modified ciphertext accepted")
	}
	if _, e = os.Stat(out); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("modified archive produced output")
	}
}
func TestExtractRejectsUnsafeAndDuplicatePaths(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:/absolute", "a\\..\\escape", "a/../escape", "NUL.txt", "bad:stream", "a./file"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "bad.zip")
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			f, e := w.Create(name)
			if e != nil {
				t.Fatal(e)
			}
			f.Write([]byte("bad"))
			w.Close()
			put(t, input, b.Bytes())
			if e = Extract(context.Background(), input, filepath.Join(dir, "out"), "", nil); e == nil {
				t.Fatal("unsafe path allowed")
			}
		})
	}
	for _, special := range []bool{false, true} {
		dir := t.TempDir()
		input := filepath.Join(dir, "bad.zip")
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		if special {
			h := &zip.FileHeader{Name: "link"}
			h.SetMode(os.ModeSymlink | 0777)
			f, _ := w.CreateHeader(h)
			f.Write([]byte("../outside"))
		} else {
			for i := 0; i < 2; i++ {
				f, _ := w.Create("same")
				f.Write([]byte("data"))
			}
		}
		w.Close()
		put(t, input, b.Bytes())
		if e := Extract(context.Background(), input, filepath.Join(dir, "out"), "", nil); e == nil {
			t.Fatal("duplicate or symlink accepted")
		}
	}
}
func TestLegacyJPGDirectExtract(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	put(t, filepath.Join(source, "你好.txt"), []byte("legacy-compatible"))
	cover := filepath.Join(dir, "cover.jpg")
	jpegFile(t, cover)
	o := Options{Source: source, Cover: cover, Output: filepath.Join(dir, "legacy.jpg")}
	p, _ := Scan(context.Background(), o)
	if e := Pack(context.Background(), o, p, nil); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(dir, "restored")
	if e := Extract(context.Background(), o.Output, out, "", nil); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(out, "你好.txt"))
	if e != nil || string(b) != "legacy-compatible" {
		t.Fatal("legacy extraction failed")
	}
}

func TestIndependentAESFixtures(t *testing.T) {
	for _, name := range []string{"pyzipper-aes128.zip", "pyzipper-aes192.zip", "pyzipper-aes256.zip"} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "result")
			if err := Extract(context.Background(), filepath.Join("testdata", name), out, "测试-password!", nil); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(out, "nested", "你好.txt"))
			if err != nil || string(data) != "Independent AES fixture ✓" {
				t.Fatalf("independent fixture: %q %v", data, err)
			}
		})
	}
}

func TestCancelDuringExtractionCleansStaging(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	put(t, filepath.Join(source, "large"), make([]byte, 1024*1024))
	o := Options{Source: source, Output: filepath.Join(dir, "encrypted.zip"), Password: "password"}
	p, _ := Scan(context.Background(), o)
	if err := Pack(context.Background(), o, p, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := filepath.Join(dir, "result")
	err := Extract(ctx, o.Output, out, o.Password, func(int64, string) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("not cancelled: %v", err)
	}
	if _, err = os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled output remains")
	}
	stages, _ := filepath.Glob(filepath.Join(dir, ".easypackage-*"))
	if len(stages) != 0 {
		t.Fatalf("staging remains: %v", stages)
	}
}
