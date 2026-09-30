package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func entries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	r, e := zip.OpenReader(path)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	result := map[string][]byte{}
	for _, f := range r.File {
		in, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(in)
		in.Close()
		if e != nil {
			t.Fatal(e)
		}
		result[f.Name] = b
	}
	return result
}
func jpegFile(t *testing.T, path string) {
	t.Helper()
	var b bytes.Buffer
	if e := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 32, 24)), nil); e != nil {
		t.Fatal(e)
	}
	put(t, path, b.Bytes())
}
func noTemps(t *testing.T, dir string) {
	t.Helper()
	matches, e := filepath.Glob(filepath.Join(dir, ".easypackage-*.tmp"))
	if e != nil || len(matches) > 0 {
		t.Fatalf("temporary files remain: %v %v", matches, e)
	}
}

func TestFiltersAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "中文项目")
	os.MkdirAll(filepath.Join(source, "空目录"), 0755)
	put(t, filepath.Join(source, "你好.java"), []byte("中文内容"))
	put(t, filepath.Join(source, "App.CLASS"), []byte("compiled"))
	put(t, filepath.Join(source, "exact.bin"), make([]byte, 1024))
	put(t, filepath.Join(source, "large.bin"), make([]byte, 1025))
	put(t, filepath.Join(source, ".git", "config"), []byte("retain"))
	out := filepath.Join(source, "结果.zip")
	put(t, out, []byte("old"))
	o := Options{Source: source, Output: out, FilterClass: true, MaxBytes: 1024, Overwrite: true}
	plan, e := Scan(ctx, o)
	if e != nil {
		t.Fatal(e)
	}
	if plan.Files != 2 || plan.Filtered != 3 || plan.ExcludedDirs != 1 {
		t.Fatalf("bad scan: %+v", plan)
	}
	if e = Pack(ctx, o, plan, nil); e != nil {
		t.Fatal(e)
	}
	files := entries(t, out)
	if string(files["你好.java"]) != "中文内容" || len(files["exact.bin"]) != 1024 {
		t.Fatal("contents not preserved")
	}
	if _, exists := files[".git/config"]; exists {
		t.Fatal("excluded directory archived")
	}
	if _, ok := files["空目录/"]; !ok {
		t.Fatal("empty directory lost")
	}
	if _, ok := files["App.CLASS"]; ok {
		t.Fatal("class not filtered")
	}
	if _, ok := files["结果.zip"]; ok {
		t.Fatal("output archived itself")
	}
	cover := filepath.Join(dir, "cover.jpg")
	jpegFile(t, cover)
	o.Cover = cover
	o.Output = filepath.Join(dir, "bundle.jpg")
	o.FilterClass = false
	o.MaxBytes = 0
	o.Overwrite = false
	plan, e = Scan(ctx, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = Pack(ctx, o, plan, nil); e != nil {
		t.Fatal(e)
	}
	in, e := os.Open(o.Output)
	if e != nil {
		t.Fatal(e)
	}
	_, e = jpeg.Decode(in)
	in.Close()
	if e != nil {
		t.Fatalf("JPEG not viewable: %v", e)
	}
	restored := filepath.Join(dir, "restored.zip")
	if e = Restore(ctx, o.Output, restored, false); e != nil {
		t.Fatal(e)
	}
	files = entries(t, restored)
	if string(files["App.CLASS"]) != "compiled" || len(files["large.bin"]) != 1025 {
		t.Fatal("filters off not honored")
	}
	noTemps(t, source)
	noTemps(t, dir)
}
func TestCancellationAndChangedSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	file := filepath.Join(source, "data.bin")
	put(t, file, make([]byte, 1024*1024))
	out := filepath.Join(dir, "out.zip")
	put(t, out, []byte("existing output"))
	o := Options{Source: source, Output: out, Overwrite: true}
	plan, e := Scan(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e = Pack(ctx, o, plan, func(int64, string) { cancel() })
	if !errors.Is(e, context.Canceled) {
		t.Fatalf("expected cancellation: %v", e)
	}
	b, _ := os.ReadFile(out)
	if string(b) != "existing output" {
		t.Fatal("cancel overwrote output")
	}
	noTemps(t, dir)
	put(t, file, []byte("changed"))
	if e = Pack(context.Background(), o, plan, nil); e == nil {
		t.Fatal("changed source accepted")
	}
	b, _ = os.ReadFile(out)
	if string(b) != "existing output" {
		t.Fatal("failure overwrote output")
	}
	noTemps(t, dir)
	o.Overwrite = false
	plan, _ = Scan(context.Background(), o)
	if e = Pack(context.Background(), o, plan, nil); e == nil {
		t.Fatal("overwrite without authorization")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, e = Scan(cancelled, o); !errors.Is(e, context.Canceled) {
		t.Fatal("scan cancellation ignored")
	}
}
func TestEmptyProjectAndLinks(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "empty")
	os.Mkdir(source, 0755)
	o := Options{Source: source, Output: filepath.Join(dir, "empty.zip")}
	p, e := Scan(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if e = Pack(context.Background(), o, p, nil); e != nil {
		t.Fatal(e)
	}
	if len(entries(t, o.Output)) != 0 {
		t.Fatal("empty archive")
	}
	outside := filepath.Join(dir, "private.txt")
	put(t, outside, []byte("outside"))
	if e = os.Symlink(outside, filepath.Join(source, "link.txt")); e != nil {
		t.Log("symlink test skipped: OS privilege unavailable")
		return
	}
	p, e = Scan(context.Background(), o)
	if e != nil || p.Links != 1 || p.Files != 0 {
		t.Fatalf("link handling: %+v %v", p, e)
	}
}
func TestRestoreValidatesCRCAndPreservesOutput(t *testing.T) {
	dir := t.TempDir()
	cover := filepath.Join(dir, "cover.jpg")
	jpegFile(t, cover)
	imageBytes, _ := os.ReadFile(cover)
	// This fixture uses the former Java edition's exact footer layout and ZIP-relative offsets.
	var payload bytes.Buffer
	zw := zip.NewWriter(&payload)
	writer, e := zw.CreateHeader(&zip.FileHeader{Name: "a.txt", Method: zip.Store})
	if e != nil {
		t.Fatal(e)
	}
	writer.Write([]byte("INTEGRITY-CHECK"))
	zw.Close()
	var packed bytes.Buffer
	packed.Write(imageBytes)
	packed.Write(payload.Bytes())
	binary.Write(&packed, binary.BigEndian, uint64(len(imageBytes)))
	packed.WriteString(magic)
	jpg := filepath.Join(dir, "legacy.jpg")
	put(t, jpg, packed.Bytes())
	out := filepath.Join(dir, "restored.zip")
	if e = Restore(context.Background(), jpg, out, false); e != nil {
		t.Fatal(e)
	}
	if string(entries(t, out)["a.txt"]) != "INTEGRITY-CHECK" {
		t.Fatal("legacy format failed")
	}
	original, _ := os.ReadFile(out)
	corrupt := append([]byte(nil), packed.Bytes()...)
	idx := bytes.Index(corrupt, []byte("INTEGRITY-CHECK"))
	if idx < 0 {
		t.Fatal("fixture")
	}
	corrupt[idx] ^= 1
	put(t, jpg, corrupt)
	if e = Restore(context.Background(), jpg, out, true); e == nil {
		t.Fatal("corrupt CRC accepted")
	}
	after, _ := os.ReadFile(out)
	if !bytes.Equal(original, after) {
		t.Fatal("corruption replaced output")
	}
	if e = Restore(context.Background(), cover, out, true); e == nil {
		t.Fatal("plain JPEG accepted")
	}
	if e = Restore(context.Background(), jpg, jpg, true); e == nil {
		t.Fatal("self overwrite accepted")
	}
	noTemps(t, dir)
}
func TestInvalidCoverAndPaths(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	put(t, filepath.Join(source, "x"), []byte("ok"))
	cover := filepath.Join(dir, "bad.jpg")
	put(t, cover, []byte("not a jpeg"))
	out := filepath.Join(dir, "out.jpg")
	o := Options{Source: source, Output: out, Cover: cover}
	p, e := Scan(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if e = Pack(context.Background(), o, p, nil); e == nil {
		t.Fatal("invalid JPEG accepted")
	}
	if _, e = os.Stat(out); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("invalid output published")
	}
	noTemps(t, dir)
	o.Output = cover
	o.Overwrite = true
	if e = Pack(context.Background(), o, p, nil); e == nil {
		t.Fatal("cover overwrite accepted")
	}
	o.Output = filepath.Join(dir, "missing", "x.zip")
	if _, e = Scan(context.Background(), o); e == nil {
		t.Fatal("invalid destination accepted")
	}
	o.Output = out
	o.Source = cover
	if _, e = Scan(context.Background(), o); e == nil {
		t.Fatal("file source accepted")
	}
}
