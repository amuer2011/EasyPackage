package archive

// WinZip AES specification (AE-2), implemented with Go's standard cryptography.
// https://www.winzip.com/en/support/aes-encryption/
// The format mandates PBKDF2-HMAC-SHA1/1000, a little-endian AES counter starting
// at 1, and a 10-byte HMAC-SHA1 over the ciphertext. It is NOT Go's big-endian CTR.
import (
	"archive/zip"
	"compress/flate"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

type littleCTR struct {
	block   cipher.Block
	counter [16]byte
	stream  [16]byte
	offset  int
}

func newLittleCTR(key []byte) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c := &littleCTR{block: block, offset: 16}
	c.counter[0] = 1
	return c, nil
}
func (c *littleCTR) XORKeyStream(dst, src []byte) {
	if len(dst) < len(src) {
		panic("short AES output")
	}
	for i, b := range src {
		if c.offset == 16 {
			c.block.Encrypt(c.stream[:], c.counter[:])
			for j := 0; j < 16; j++ {
				c.counter[j]++
				if c.counter[j] != 0 {
					break
				}
			}
			c.offset = 0
		}
		dst[i] = b ^ c.stream[c.offset]
		c.offset++
	}
}

func writeEncryptedEntry(ctx context.Context, zw *zip.Writer, header *zip.FileHeader, password, tempDir string, copySource func(io.Writer) error) error {
	// Spooling one compressed entry supplies its size to ZIP CreateRaw and keeps
	// memory bounded for large files. No plaintext spool is ever published.
	spool, err := os.CreateTemp(tempDir, ".easypackage-aes-*.tmp")
	if err != nil {
		return err
	}
	defer func() { spool.Close(); os.Remove(spool.Name()) }()
	compressor, err := flate.NewWriter(spool, flate.DefaultCompression)
	if err != nil {
		return err
	}
	copyErr := copySource(compressor)
	closeErr := compressor.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	size, err := spool.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return err
	}
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return err
	}
	keys, err := pbkdf2.Key(sha1.New, password, salt, 1000, 66)
	if err != nil {
		return err
	}
	header.Method = 99
	header.Flags = 1 | 0x800
	header.ReaderVersion = 51
	header.CRC32 = 0
	header.CompressedSize64 = uint64(size) + 28 // salt + password verifier + authentication
	header.Extra = append(header.Extra, 0x01, 0x99, 7, 0, 2, 0, 'A', 'E', 3, 8, 0)
	dest, err := zw.CreateRaw(header)
	if err != nil {
		return err
	}
	if _, err = dest.Write(salt); err != nil {
		return err
	}
	if _, err = dest.Write(keys[64:]); err != nil {
		return err
	}
	stream, err := newLittleCTR(keys[:32])
	if err != nil {
		return err
	}
	mac := hmac.New(sha1.New, keys[32:64])
	writer := cipher.StreamWriter{S: stream, W: io.MultiWriter(dest, mac)}
	if _, err = io.Copy(writer, contextReader{ctx, spool}); err != nil {
		return err
	}
	_, err = dest.Write(mac.Sum(nil)[:10])
	return err
}

type aesParameters struct {
	version, method     uint16
	keyBytes, saltBytes int
}

func aesInfo(f *zip.File) (aesParameters, error) {
	var result aesParameters
	found := false
	for extra := f.Extra; len(extra) > 0; {
		if len(extra) < 4 {
			return result, errors.New("ZIP 扩展字段损坏")
		}
		n := int(binary.LittleEndian.Uint16(extra[2:4]))
		if n > len(extra)-4 {
			return result, errors.New("ZIP 扩展字段损坏")
		}
		if binary.LittleEndian.Uint16(extra[:2]) == 0x9901 {
			if found || n != 7 {
				return result, errors.New("AES ZIP 信息损坏")
			}
			found = true
			data := extra[4 : 4+n]
			result.version = binary.LittleEndian.Uint16(data[:2])
			result.method = binary.LittleEndian.Uint16(data[5:7])
			if (result.version != 1 && result.version != 2) || string(data[2:4]) != "AE" || data[4] < 1 || data[4] > 3 {
				return result, errors.New("不支持此 AES ZIP 格式")
			}
			result.keyBytes = 8 + 8*int(data[4])
			result.saltBytes = result.keyBytes / 2
		}
		extra = extra[4+n:]
	}
	if !found {
		return result, errors.New("缺少 AES ZIP 加密信息")
	}
	if result.method != zip.Store && result.method != zip.Deflate {
		return result, errors.New("不支持此 ZIP 压缩方法")
	}
	return result, nil
}

// copyEntry authenticates encrypted data before decompressing it to the private
// extraction directory. Both plaintext size and checksum are then validated.
func copyEntry(ctx context.Context, f *zip.File, password string, out io.Writer) error {
	if f.Flags&1 == 0 {
		if f.Method == 99 {
			return errors.New("ZIP 加密标记损坏")
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		_, err = io.Copy(out, contextReader{ctx, r})
		return err
	}
	if password == "" {
		return errors.New("此压缩包已加密，请输入密码")
	}
	if f.Method != 99 {
		return errors.New("仅支持 AES ZIP 密码，不支持传统 ZipCrypto 格式")
	}
	parameters, err := aesInfo(f)
	if err != nil {
		return err
	}
	overhead := uint64(parameters.saltBytes + 12)
	if f.CompressedSize64 < overhead || f.CompressedSize64 > uint64(^uint64(0)>>1) {
		return errors.New("AES ZIP 长度无效")
	}
	ciphertextSize := int64(f.CompressedSize64 - overhead)
	raw, err := f.OpenRaw()
	if err != nil {
		return err
	}
	salt := make([]byte, parameters.saltBytes)
	if _, err = io.ReadFull(raw, salt); err != nil {
		return err
	}
	verifier := make([]byte, 2)
	if _, err = io.ReadFull(raw, verifier); err != nil {
		return err
	}
	keys, err := pbkdf2.Key(sha1.New, password, salt, 1000, parameters.keyBytes*2+2)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(verifier, keys[len(keys)-2:]) != 1 {
		return errors.New("密码不正确或压缩包已损坏")
	}
	mac := hmac.New(sha1.New, keys[parameters.keyBytes:parameters.keyBytes*2])
	if _, err = io.CopyN(mac, contextReader{ctx, raw}, ciphertextSize); err != nil {
		return err
	}
	tag := make([]byte, 10)
	if _, err = io.ReadFull(raw, tag); err != nil {
		return err
	}
	if !hmac.Equal(tag, mac.Sum(nil)[:10]) {
		return errors.New("密码不正确或压缩包完整性校验失败")
	}
	raw, err = f.OpenRaw()
	if err != nil {
		return err
	}
	if _, err = io.CopyN(io.Discard, raw, int64(parameters.saltBytes+2)); err != nil {
		return err
	}
	stream, err := newLittleCTR(keys[:parameters.keyBytes])
	if err != nil {
		return err
	}
	decrypted := &cipher.StreamReader{S: stream, R: contextReader{ctx, io.LimitReader(raw, ciphertextSize)}}
	var reader io.Reader = decrypted
	if parameters.method == zip.Deflate {
		r := flate.NewReader(decrypted)
		defer r.Close()
		reader = r
	}
	if f.UncompressedSize64 >= uint64(^uint64(0)>>1) {
		return errors.New("ZIP 文件大小超出范围")
	}
	crc := crc32.NewIEEE()
	n, err := io.Copy(io.MultiWriter(out, crc), contextReader{ctx, io.LimitReader(reader, int64(f.UncompressedSize64)+1)})
	if err != nil {
		return err
	}
	if uint64(n) != f.UncompressedSize64 {
		return errors.New("解压大小与 ZIP 记录不一致")
	}
	if parameters.version == 1 && crc.Sum32() != f.CRC32 {
		return fmt.Errorf("ZIP 校验失败：%s", f.Name)
	}
	return nil
}
