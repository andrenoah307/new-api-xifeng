// Package imageresult 把成功的生图响应落地保存到本地存储，
// 应对 CDN（如 Cloudflare 橙云）非流式 120s 超时：客户端拿不到响应时，
// 用户仍可在后台"绘图日志"页取回已生成并已计费的图片。
package imageresult

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/attachment"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	// StorageRoot 生图结果的本地存储根目录，与工单附件目录隔离。
	StorageRoot = "data/image_results"

	promptMaxLen        = 1000
	urlDownloadTimeout  = 60 * time.Second
	maxImagesPerRequest = 128 // 与 dto.MaxImageN 对齐；防御上游返回异常大的 data 数组
)

// Store 返回生图结果专用的本地存储实例。
// 复用 attachment.LocalStorage 的 jail/safePath 防穿越实现，仅换根目录。
func Store() (attachment.Storage, error) {
	return attachment.NewLocalStorage(StorageRoot)
}

// MaybeSave 在生图响应写回客户端前调用：若开关开启且是成功的
// generations/edits 响应，异步把图片内容保存到本地并登记 image_results。
// 任何失败都只记日志，绝不影响主链路的响应与计费。
func MaybeSave(c *gin.Context, info *relaycommon.RelayInfo, responseBody []byte) {
	if !operation_setting.ImageResultEnabled || info == nil {
		return
	}
	if info.RelayMode != relayconstant.RelayModeImagesGenerations && info.RelayMode != relayconstant.RelayModeImagesEdits {
		return
	}
	if len(responseBody) == 0 || !gjson.GetBytes(responseBody, "data").IsArray() {
		return
	}

	mode := "generations"
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		mode = "edits"
	}
	record := &model.ImageResult{
		UserId:      info.UserId,
		TokenId:     info.TokenId,
		ModelName:   info.OriginModelName,
		Mode:        mode,
		Prompt:      extractPrompt(c),
		RequestId:   info.RequestId,
		CreatedTime: common.GetTimestamp(),
	}

	// 响应体与 gin.Context 在响应写回后都会被复用/回收，异步前必须拷贝出去。
	bodyCopy := make([]byte, len(responseBody))
	copy(bodyCopy, responseBody)

	gopool.Go(func() {
		if err := saveImages(record, bodyCopy); err != nil {
			common.SysError(fmt.Sprintf("image result save failed: user=%d model=%s request=%s err=%s",
				record.UserId, record.ModelName, record.RequestId, err.Error()))
		}
	})
}

// extractPrompt 从请求体里取 prompt（JSON 路径）；multipart（edits 常见）取不到留空。
// 必须在 MaybeSave 的同步阶段调用——请求体存储在请求结束后会被清理。
func extractPrompt(c *gin.Context) string {
	bs, err := common.GetBodyStorage(c)
	if err != nil {
		return ""
	}
	data, err := bs.Bytes()
	if err != nil {
		return ""
	}
	prompt := gjson.GetBytes(data, "prompt").String()
	if len(prompt) > promptMaxLen {
		prompt = prompt[:promptMaxLen]
	}
	return prompt
}

func saveImages(record *model.ImageResult, responseBody []byte) error {
	store, err := Store()
	if err != nil {
		return fmt.Errorf("init storage: %w", err)
	}

	items := gjson.GetBytes(responseBody, "data").Array()
	if len(items) > maxImagesPerRequest {
		items = items[:maxImagesPerRequest]
	}

	maxBytes := int64(operation_setting.ImageResultMaxFileSizeMB) * 1024 * 1024
	var files []model.ImageResultFile
	for idx, item := range items {
		var file *model.ImageResultFile
		var saveErr error
		if b64 := item.Get("b64_json").String(); b64 != "" {
			file, saveErr = saveBase64Image(store, b64, maxBytes)
		} else if imgURL := item.Get("url").String(); imgURL != "" {
			file, saveErr = downloadAndSaveImage(store, imgURL, maxBytes)
		} else {
			continue
		}
		if saveErr != nil {
			common.SysError(fmt.Sprintf("image result item skipped: request=%s idx=%d err=%s",
				record.RequestId, idx, saveErr.Error()))
			continue
		}
		files = append(files, *file)
	}

	if len(files) == 0 {
		return nil // 没有可保存的图片（或全部失败，单项错误已各自记录）
	}
	if err := record.SetFiles(files); err != nil {
		return fmt.Errorf("marshal files: %w", err)
	}
	return record.Insert()
}

func saveBase64Image(store attachment.Storage, b64 string, maxBytes int64) (*model.ImageResultFile, error) {
	// 兼容 data URI 前缀（个别渠道会带 data:image/png;base64,）
	if idx := strings.Index(b64, ";base64,"); idx >= 0 {
		b64 = b64[idx+len(";base64,"):]
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("image size %d exceeds limit %d", len(data), maxBytes)
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	return putImage(store, data, mime, "b64")
}

func downloadAndSaveImage(store attachment.Storage, imgURL string, maxBytes int64) (*model.ImageResultFile, error) {
	if err := service.ValidateSSRFProtectedFetchURL(imgURL); err != nil {
		return nil, fmt.Errorf("url blocked: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), urlDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := service.GetSSRFProtectedHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download status %d", resp.StatusCode)
	}
	// LimitReader 读到 maxBytes+1 说明超限，直接放弃（不存半张图）。
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("image size exceeds limit %d", maxBytes)
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "image/") {
			mime = ct
		} else {
			mime = "image/png"
		}
	}
	return putImage(store, data, mime, "url")
}

func putImage(store attachment.Storage, data []byte, mime string, source string) (*model.ImageResultFile, error) {
	ext := "png"
	switch mime {
	case "image/jpeg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	case "image/gif":
		ext = "gif"
	}
	now := time.Now()
	key := path.Join(fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", now.Month()), uuid.NewString()+"."+ext)
	if err := store.Put(context.Background(), key, bytes.NewReader(data), int64(len(data)), mime); err != nil {
		return nil, fmt.Errorf("store put: %w", err)
	}
	return &model.ImageResultFile{Key: key, Mime: mime, Size: int64(len(data)), Source: source}, nil
}
