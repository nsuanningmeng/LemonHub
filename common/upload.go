package common

import (
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder for image.DecodeConfig
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp" // register WebP decoder
)

// Image dimension caps guard against decompression / pixel-flood bombs: a tiny
// file can declare enormous dimensions that exhaust memory when decoded by a
// viewer. We validate the header only (DecodeConfig), never the full pixels.
const (
	maxImageDimension = 20000    // max width or height in pixels
	maxImagePixels    = 40000000 // max total pixels (~40 MP)
)

// uploadBaseDir resolves the root directory for user uploads. Configurable via
// the UPLOAD_PATH env var, defaulting to ./data/uploads.
func uploadBaseDir() string {
	if p := os.Getenv("UPLOAD_PATH"); p != "" {
		return p
	}
	return filepath.Join("data", "uploads")
}

// mimeToExt maps an allowed image MIME type to a safe file extension.
var mimeToExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// SavedUpload describes a persisted upload.
type SavedUpload struct {
	StoredName string // random on-disk filename (uuid + ext)
	RelPath    string // forward-slash relative path stored in DB (e.g. tickets/ab/uuid.png)
	MimeType   string // sniffed content type
	Size       int64  // bytes written
}

// SaveTicketUpload validates and persists an uploaded image for the ticket
// system. Security properties:
//   - size is capped (header + streamed copy guard),
//   - the real content type is sniffed from bytes (not trusted from the client),
//   - only whitelisted image MIME types are accepted,
//   - the on-disk filename is fully server-generated (uuid + mapped extension),
//     so the original (attacker-controlled) filename never touches the path.
func SaveTicketUpload(fileHeader *multipart.FileHeader, allowedMimes []string, maxSizeBytes int64) (*SavedUpload, error) {
	if fileHeader == nil {
		return nil, fmt.Errorf("no file provided")
	}
	if maxSizeBytes <= 0 {
		maxSizeBytes = 5 * 1024 * 1024
	}
	if fileHeader.Size > maxSizeBytes {
		return nil, fmt.Errorf("file too large")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	// Sniff content type from the first 512 bytes.
	head := make([]byte, 512)
	n, _ := io.ReadFull(src, head)
	if n == 0 {
		return nil, fmt.Errorf("empty file")
	}
	mimeType := http.DetectContentType(head[:n])
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))

	allowed := false
	for _, m := range allowedMimes {
		if strings.EqualFold(m, mimeType) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("unsupported file type: %s", mimeType)
	}
	ext, ok := mimeToExt[mimeType]
	if !ok {
		return nil, fmt.Errorf("unsupported file type: %s", mimeType)
	}

	seeker, ok := src.(io.Seeker)
	if !ok {
		return nil, fmt.Errorf("file not seekable")
	}

	// Structural image validation: confirm the bytes are a genuinely decodable image
	// of the expected format, and bound the pixel dimensions. This raises the bar past
	// the 512-byte content sniff (which is satisfied by magic bytes alone) — a file that
	// merely prefixes an image signature onto another payload fails to decode here — and
	// it rejects decompression/pixel-flood bombs. DecodeConfig reads only the header.
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	cfg, decodedFormat, err := image.DecodeConfig(src)
	if err != nil {
		return nil, fmt.Errorf("file is not a valid image")
	}
	// The decoded format must match the sniffed type (image/jpeg→"jpeg", image/png→"png",
	// image/gif→"gif", image/webp→"webp"), closing format-confusion polyglots.
	if decodedFormat != strings.TrimPrefix(mimeType, "image/") {
		return nil, fmt.Errorf("image format mismatch")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 ||
		cfg.Width > maxImageDimension || cfg.Height > maxImageDimension ||
		int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, fmt.Errorf("image dimensions not allowed")
	}

	// Rewind to the start so the full original bytes are written to disk.
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	uuid := GetUUID()
	storedName := uuid + ext
	shard := uuid[:2]
	relPath := path.Join("tickets", shard, storedName)

	base := uploadBaseDir()
	absDir := filepath.Join(base, "tickets", shard)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, err
	}
	absPath := filepath.Join(absDir, storedName)

	dst, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}

	// Guard the streamed copy against a header that under-reports size.
	written, copyErr := io.Copy(dst, io.LimitReader(src, maxSizeBytes+1))
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(absPath)
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(absPath)
		return nil, closeErr
	}
	if written > maxSizeBytes {
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("file too large")
	}

	return &SavedUpload{
		StoredName: storedName,
		RelPath:    relPath,
		MimeType:   mimeType,
		Size:       written,
	}, nil
}

// ResolveUploadPath maps a DB-stored relative path to an absolute on-disk path,
// rejecting any path that would escape the upload base directory.
func ResolveUploadPath(relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("empty path")
	}
	clean := path.Clean("/" + strings.ReplaceAll(relPath, "\\", "/"))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid path")
	}
	base := uploadBaseDir()
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absPath := filepath.Join(absBase, filepath.FromSlash(clean))
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path")
	}
	return absPath, nil
}

// DeleteUpload removes a stored upload by its DB-relative path. Missing files
// are treated as success (idempotent cleanup).
func DeleteUpload(relPath string) error {
	absPath, err := ResolveUploadPath(relPath)
	if err != nil {
		return err
	}
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
