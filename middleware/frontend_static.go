package middleware

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	ginGzip "github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// precompressedFile is the gzip encoding of one frontend build file.
type precompressedFile struct {
	once        sync.Once
	body        []byte
	contentType string
	modified    time.Time
	err         error
}

// ServeFrontendFiles serves files from the embedded frontend build. The build
// never changes while the process runs, so each file is gzip-compressed once
// and the bytes are reused, instead of recompressing multi-megabyte chunks on
// every request. The cache holds at most one entry per build file. Clients
// without gzip, the image types the gzip middleware skips, directories, and
// the index.html redirect keep using http.FileServer.
func ServeFrontendFiles(frontendFS static.ServeFileSystem) gin.HandlerFunc {
	fileServer := http.FileServer(frontendFS)
	var cache sync.Map // URL path -> *precompressedFile
	return func(c *gin.Context) {
		urlPath := path.Clean("/" + c.Request.URL.Path)
		if !frontendFS.Exists("/", urlPath) {
			return
		}
		c.Abort()
		// Vary applies to both the compressed and identity representation.
		c.Writer.Header().Add("Vary", "Accept-Encoding")
		acceptsGzip := false
		for _, encoding := range strings.Split(c.GetHeader("Accept-Encoding"), ",") {
			parts := strings.Split(encoding, ";")
			if !strings.EqualFold(strings.TrimSpace(parts[0]), "gzip") {
				continue
			}
			acceptsGzip = true
			for _, parameter := range parts[1:] {
				name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
				if ok && strings.EqualFold(name, "q") {
					quality, err := strconv.ParseFloat(value, 64)
					acceptsGzip = err == nil && quality > 0 && quality <= 1
				}
			}
			break
		}
		if !acceptsGzip ||
			ginGzip.DefaultExcludedExtentions.Contains(path.Ext(urlPath)) ||
			strings.HasSuffix(urlPath, "/index.html") || strings.HasSuffix(c.Request.URL.Path, "/") {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		value, ok := cache.Load(urlPath)
		if !ok {
			value, _ = cache.LoadOrStore(urlPath, &precompressedFile{})
		}
		file := value.(*precompressedFile)
		file.once.Do(func() {
			f, err := frontendFS.Open(urlPath)
			if err != nil {
				file.err = err
				return
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil {
				file.err = err
				return
			}
			if info.IsDir() {
				file.err = errors.New("directory")
				return
			}
			raw, err := io.ReadAll(f)
			if err != nil {
				file.err = err
				return
			}
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			if _, err := writer.Write(raw); err != nil {
				file.err = err
				return
			}
			if err := writer.Close(); err != nil {
				file.err = err
				return
			}
			// Match http.FileServer: extension first, then content sniffing.
			file.contentType = mime.TypeByExtension(path.Ext(urlPath))
			if file.contentType == "" {
				file.contentType = http.DetectContentType(raw)
			}
			file.body = bytes.Clone(compressed.Bytes())
			file.modified = info.ModTime()
		})
		if file.err != nil {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		header := c.Writer.Header()
		header.Set("Content-Encoding", "gzip")
		header.Set("Content-Type", file.contentType)
		// ServeContent preserves conditional requests and HEAD semantics for the
		// compressed representation, just as FileServer does for plain bytes.
		http.ServeContent(c.Writer, c.Request, path.Base(urlPath), file.modified, bytes.NewReader(file.body))
	}
}

// IsFrontendBuildAsset distinguishes actual build assets from HTML entry points
// and directory redirects, which must retain the page request budget.
func IsFrontendBuildAsset(frontendFS static.ServeFileSystem, requestPath string) bool {
	cleanPath := path.Clean("/" + requestPath)
	if cleanPath == "/" || strings.HasSuffix(requestPath, "/") || strings.HasSuffix(cleanPath, "/index.html") || !frontendFS.Exists("/", cleanPath) {
		return false
	}
	file, err := frontendFS.Open(cleanPath)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	return err == nil && !info.IsDir()
}
