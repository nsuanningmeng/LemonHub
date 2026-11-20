package common

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReusableJSONBodyRejectsTrailingValuesInMemoryAndOnDisk(t *testing.T) {
	previous := GetDiskCacheConfig()
	t.Cleanup(func() { SetDiskCacheConfig(previous) })
	for _, contentType := range []string{"application/json", "multipart/form-data"} {
		for _, disk := range []bool{false, true} {
			for _, suffix := range []string{" \n\t", " {}", " null", " garbage"} {
				mode := "memory"
				if disk {
					mode = "disk"
				}
				t.Run(contentType+"/"+mode+"/"+suffix, func(t *testing.T) {
					SetDiskCacheConfig(DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 16, Path: t.TempDir()})
					payload := `{"n":0,"enabled":false}` + suffix
					storage, err := CreateBodyStorage([]byte(payload))
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, storage.Close()) })
					require.Equal(t, disk, storage.IsDisk())
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/body", strings.NewReader(payload))
					c.Request.Header.Set("Content-Type", contentType)
					c.Set(KeyBodyStorage, storage)
					var decoded struct {
						N       *uint `json:"n"`
						Enabled *bool `json:"enabled"`
					}
					err = UnmarshalBodyReusable(c, &decoded)
					if strings.TrimSpace(suffix) != "" {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					require.NotNil(t, decoded.N)
					assert.Zero(t, *decoded.N)
					require.NotNil(t, decoded.Enabled)
					assert.False(t, *decoded.Enabled)
					replay, err := io.ReadAll(c.Request.Body)
					require.NoError(t, err)
					assert.Equal(t, payload, string(replay))
				})
			}
		}
	}
}

func TestMultipartBodyReplaysOriginalBoundaryFilesAndFields(t *testing.T) {
	previous := GetDiskCacheConfig()
	previousLimit := constant.MaxFileDownloadMB
	t.Cleanup(func() { SetDiskCacheConfig(previous); constant.MaxFileDownloadMB = previousLimit })
	constant.MaxFileDownloadMB = 1
	contents := bytes.Repeat([]byte("x"), (1<<20)+17)
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	require.NoError(t, writer.WriteField("tag", "first"))
	require.NoError(t, writer.WriteField("tag", "second"))
	part, err := writer.CreateFormFile("image", "image.png")
	require.NoError(t, err)
	_, err = part.Write(contents)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	for _, disk := range []bool{false, true} {
		mode := "memory"
		if disk {
			mode = "disk"
		}
		t.Run(mode, func(t *testing.T) {
			SetDiskCacheConfig(DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 16, Path: t.TempDir()})
			storage, err := CreateBodyStorage(payload.Bytes())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, storage.Close()) })
			require.Equal(t, disk, storage.IsDisk())
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/body", bytes.NewReader(payload.Bytes()))
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			c.Set(KeyBodyStorage, storage)
			form, err := ParseMultipartFormReusable(c)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, form.RemoveAll()) })
			assert.Equal(t, []string{"first", "second"}, form.Value["tag"])
			require.Len(t, form.File["image"], 1)
			file, err := form.File["image"][0].Open()
			require.NoError(t, err)
			actual, err := io.ReadAll(file)
			require.NoError(t, err)
			require.NoError(t, file.Close())
			assert.Equal(t, contents, actual)
			c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=outbound")
			var fields map[string]any
			require.NoError(t, UnmarshalBodyReusable(c, &fields))
			assert.Equal(t, map[string]any{"tag": []any{"first", "second"}}, fields)
			replay, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			assert.Equal(t, payload.Bytes(), replay)
		})
	}
}
