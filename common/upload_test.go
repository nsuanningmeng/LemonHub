package common

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// onePixelPNGBase64 is a valid 1x1 PNG. Decoded it is a genuine, decodable image
// (passes image.DecodeConfig), unlike a magic-bytes-only forgery.
const onePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// newUploadFileHeader builds a real *multipart.FileHeader by writing a multipart
// form to a buffer and parsing it, mirroring how Gin produces file headers.
func newUploadFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	r := multipart.NewReader(&buf, w.Boundary())
	form, err := r.ReadForm(int64(len(content)) + 4096)
	require.NoError(t, err)
	files := form.File["file"]
	require.Len(t, files, 1)
	return files[0]
}

func TestSaveTicketUpload_ValidatesContent(t *testing.T) {
	t.Setenv("UPLOAD_PATH", t.TempDir())

	pngBytes, err := base64.StdEncoding.DecodeString(onePixelPNGBase64)
	require.NoError(t, err)
	require.NotEmpty(t, pngBytes)

	// A file that only prefixes the PNG magic bytes onto an HTML payload: it sniffs
	// as image/png but is NOT a decodable image.
	fakePNG := append([]byte("\x89PNG\r\n\x1a\n"), []byte("<html><body>not an image</body></html>")...)

	t.Run("valid png is saved", func(t *testing.T) {
		fh := newUploadFileHeader(t, "evil.php.png", pngBytes)
		saved, err := SaveTicketUpload(fh, []string{"image/png"}, 5*1024*1024)
		require.NoError(t, err)
		require.NotNil(t, saved)

		assert.Equal(t, "image/png", saved.MimeType)
		assert.True(t, strings.HasSuffix(saved.RelPath, ".png"), "RelPath %q must end in .png", saved.RelPath)
		assert.True(t, strings.HasSuffix(saved.StoredName, ".png"), "StoredName %q must end in .png", saved.StoredName)
		// The attacker-controlled original filename must never appear in the stored path.
		assert.NotContains(t, saved.RelPath, "evil")
		assert.NotContains(t, saved.RelPath, ".php")
		// RelPath is the forward-slash sharded path under tickets/.
		assert.True(t, strings.HasPrefix(saved.RelPath, "tickets/"), "RelPath %q must be under tickets/", saved.RelPath)
		assert.Equal(t, int64(len(pngBytes)), saved.Size)

		// The bytes actually landed on disk and resolve back to a contained path.
		abs, err := ResolveUploadPath(saved.RelPath)
		require.NoError(t, err)
		_, statErr := os.Stat(abs)
		require.NoError(t, statErr)
	})

	t.Run("magic-bytes-only forgery is rejected", func(t *testing.T) {
		fh := newUploadFileHeader(t, "fake.png", fakePNG)
		saved, err := SaveTicketUpload(fh, []string{"image/png"}, 5*1024*1024)
		require.Error(t, err)
		assert.Nil(t, saved)
	})

	t.Run("plain text is rejected as unsupported type", func(t *testing.T) {
		fh := newUploadFileHeader(t, "note.txt", []byte("just some plain text, not an image at all"))
		saved, err := SaveTicketUpload(fh, []string{"image/png"}, 5*1024*1024)
		require.Error(t, err)
		assert.Nil(t, saved)
	})

	t.Run("oversize file is rejected", func(t *testing.T) {
		fh := newUploadFileHeader(t, "big.png", pngBytes)
		// maxSizeBytes far below the PNG size.
		saved, err := SaveTicketUpload(fh, []string{"image/png"}, 10)
		require.Error(t, err)
		assert.Nil(t, saved)
	})

	t.Run("nil file header is rejected", func(t *testing.T) {
		saved, err := SaveTicketUpload(nil, []string{"image/png"}, 5*1024*1024)
		require.Error(t, err)
		assert.Nil(t, saved)
	})
}

// TestResolveUploadPath_NoEscape verifies the path-traversal defense. The function
// anchors the relative path at "/" then path.Clean's it, so traversal segments are
// NEUTRALIZED (the result stays inside the base) rather than producing an error.
// The genuine security contract is: the resolved path never escapes the base dir.
func TestResolveUploadPath_NoEscape(t *testing.T) {
	base := t.TempDir()
	t.Setenv("UPLOAD_PATH", base)
	absBase, err := filepath.Abs(base)
	require.NoError(t, err)

	t.Run("empty path is rejected", func(t *testing.T) {
		_, err := ResolveUploadPath("")
		require.Error(t, err)
	})

	t.Run("normal path resolves under base", func(t *testing.T) {
		got, err := ResolveUploadPath("tickets/ab/uuid.png")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(got, absBase), "%q must be under base %q", got, absBase)
		assert.True(t, strings.HasSuffix(got, "uuid.png"))
	})

	// Traversal inputs must never escape the base directory. They are contained, not
	// errored, because the implementation neutralizes the traversal.
	traversal := []string{
		"../../etc/passwd",
		"..\\..\\x",
		"a/../../../etc",
		"tickets/../../../etc/passwd",
	}
	for _, in := range traversal {
		t.Run("contained:"+in, func(t *testing.T) {
			got, err := ResolveUploadPath(in)
			require.NoError(t, err)
			rel, relErr := filepath.Rel(absBase, got)
			require.NoError(t, relErr)
			assert.False(t, rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)),
				"input %q escaped base: rel=%q", in, rel)
			assert.True(t, strings.HasPrefix(got, absBase), "input %q escaped base: %q", in, got)
		})
	}

	// A literal ".." substring that survives Clean (a filename beginning with "..")
	// hits the explicit rejection branch.
	t.Run("literal dotdot name is rejected", func(t *testing.T) {
		_, err := ResolveUploadPath("..foo")
		require.Error(t, err)
	})
}
