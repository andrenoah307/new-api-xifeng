package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/attachment"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Attachment controller 提供工单附件的上传 / 预览下载 / 撤回删除三个端点。
//
// 约束：
//   - 所有请求必须先过 UserAuth（路由层配置），controller 内不再做登录验证；
//   - 权限分级靠 role + UserId 校对：
//     * 上传：当前用户即归属者；
//     * 下载：必须属于同一工单（用户）或 role >= RoleAdminUser（管理员）；
//     * 删除：仅上传者本人，且附件尚未绑定到消息（MessageId == 0）。

type uploadAttachmentResponse struct {
	Id          int    `json:"id"`
	FileName    string `json:"file_name"`
	MimeType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	Sha256      string `json:"sha256"`
	StorageType string `json:"storage_type"`
	Previewable bool   `json:"previewable"`
	CreatedTime int64  `json:"created_time"`
}

// UploadTicketAttachment 处理 multipart 上传。
// 约定：
//   - 只接受单文件（form 字段名 "file"）。前端多文件通过多次上传拼 attachment_ids[]。
//   - 服务端以 UUID + ext 生成 storedName；上传者/尺寸/类型全部重新嗅探判定。
//   - 上传成功后返回 id 与可预览标识，供前端在发消息时携带。
func UploadTicketAttachment(c *gin.Context) {
	if !setting.TicketAttachmentEnabled {
		common.ApiErrorMsg(c, "ticket attachment is disabled")
		return
	}
	userId := c.GetInt("id")
	if userId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgForbidden)
		return
	}

	// 先限制请求体大小，防御超大 multipart 耗尽磁盘/内存。
	// 上浮 10% 给 multipart 边界与表单字段开销。
	maxBody := setting.TicketAttachmentMaxSize + (setting.TicketAttachmentMaxSize / 10)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			common.ApiErrorMsg(c, fmt.Sprintf("file too large, max %d bytes", setting.TicketAttachmentMaxSize))
			return
		}
		common.ApiErrorMsg(c, "missing file")
		return
	}
	if fileHeader.Size <= 0 {
		common.ApiErrorMsg(c, "empty file")
		return
	}
	if fileHeader.Size > setting.TicketAttachmentMaxSize {
		common.ApiErrorMsg(c, fmt.Sprintf("file too large, max %d bytes", setting.TicketAttachmentMaxSize))
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer src.Close()

	// Sniff content before checking extension/MIME allowlists, then rebuild the stream.
	head, combined, err := attachment.ReadSniff(src)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := attachment.Validate(fileHeader.Filename, "", fileHeader.Size, head)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	storage, err := attachment.Current()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// storedName = uuid + "." + ext；所有客户端输入都不进入路径。
	storedName := uuid.NewString()
	if result.Ext != "" {
		storedName += "." + result.Ext
	}
	// 本地存储按年月分目录，减少单目录文件数。云存储同样适用（无额外费用且方便审计）。
	now := time.Now().UTC()
	key := path.Join(fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", now.Month()), storedName)

	// 边写盘边算 sha256，存 DB 便于去重与校验。
	hasher := sha256.New()
	teeReader := io.TeeReader(combined, hasher)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	if err := storage.Put(ctx, key, teeReader, fileHeader.Size, result.DetectedMime); err != nil {
		common.ApiError(c, err)
		return
	}

	record := &model.TicketAttachment{
		TicketId:    0,
		MessageId:   0,
		UserId:      userId,
		FileName:    result.SafeName,
		StoredName:  storedName,
		MimeType:    result.DetectedMime,
		Size:        fileHeader.Size,
		StorageType: storage.Kind(),
		StorageKey:  key,
		Sha256:      hex.EncodeToString(hasher.Sum(nil)),
	}
	if err := model.CreateTicketAttachment(record); err != nil {
		// 失败时尽量回滚文件，避免遗留孤儿。
		_ = storage.Delete(context.Background(), key)
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, uploadAttachmentResponse{
		Id:          record.Id,
		FileName:    record.FileName,
		MimeType:    record.MimeType,
		Size:        record.Size,
		Sha256:      record.Sha256,
		StorageType: record.StorageType,
		Previewable: isPreviewableMime(record.MimeType),
		CreatedTime: record.CreatedTime,
	})
}

// DeleteTicketAttachment 撤回尚未绑定到消息的附件（仅上传者本人）。
// 已绑定的附件不允许删除，避免破坏工单消息完整性。
func DeleteTicketAttachment(c *gin.Context) {
	userId := c.GetInt("id")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	record, err := model.GetTicketAttachmentById(id)
	if err != nil {
		if errors.Is(err, model.ErrAttachmentNotFound) {
			common.ApiErrorMsg(c, "attachment not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	if record.UserId != userId {
		common.ApiErrorI18n(c, i18n.MsgForbidden)
		return
	}
	if record.MessageId != 0 {
		common.ApiErrorMsg(c, "attachment already bound to a message")
		return
	}

	storage, err := attachment.Current()
	if err == nil {
		_ = storage.Delete(c.Request.Context(), record.StorageKey)
	}
	if err := model.DeleteAttachment(record.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": record.Id})
}

// DownloadTicketAttachment 提供预览/下载入口。
//   - 管理员可访问任何附件；
//   - 普通用户仅能访问自己工单的附件；
//   - 未绑定到消息的附件仅上传者本人可以访问（用于前端预览刚上传的文件）。
//   - 云存储经签名 URL 302 跳转，不穿透后端带宽；
//   - 本地存储直接由后端流式返回，文件永不暴露到静态路径。
//   - 图片/文本类以 inline 打开，其它附件强制 attachment 下载。
func DownloadTicketAttachment(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	record, err := model.GetTicketAttachmentById(id)
	if err != nil {
		if errors.Is(err, model.ErrAttachmentNotFound) {
			common.ApiErrorMsg(c, "attachment not found")
			return
		}
		common.ApiError(c, err)
		return
	}

	if !canAccessAttachment(c, record) {
		common.ApiErrorI18n(c, i18n.MsgForbidden)
		return
	}

	storage, err := attachment.Current()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Proxy every backend so legacy object metadata cannot bypass response safety.
	reader, err := storage.Get(c.Request.Context(), record.StorageKey)
	if err != nil {
		if errors.Is(err, attachment.ErrNotFound) {
			common.ApiErrorMsg(c, "attachment content missing")
			return
		}
		common.ApiError(c, err)
		return
	}
	defer reader.Close()

	head, combined, err := attachment.ReadSniff(reader)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	contentType := strings.Split(attachment.DetectMime(head), ";")[0]
	previewable := isPreviewableMime(contentType)
	if !previewable {
		contentType = "application/octet-stream"
	}
	disposition := "attachment"
	if c.Query("inline") == "1" && previewable {
		disposition = "inline"
	}
	dispHeader := fmt.Sprintf(`%s; filename*=UTF-8''%s`, disposition, url.PathEscape(record.FileName))
	c.Writer.Header().Set("Content-Type", contentType)
	c.Writer.Header().Set("Content-Disposition", dispHeader)
	c.Writer.Header().Set("Content-Length", strconv.FormatInt(record.Size, 10))
	// 显式关闭嗅探，杜绝浏览器把 txt 当成 HTML 执行。
	c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
	c.Writer.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; base-uri 'none'; form-action 'none'")
	c.Writer.Header().Set("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, combined); err != nil {
		// 已经写了响应头，这里只能记录；客户端会看到截断的连接。
		common.SysLog("ticket attachment download: copy failed: " + err.Error())
	}
}

// canAccessAttachment 判断当前用户是否有权访问给定附件。
func canAccessAttachment(c *gin.Context, a *model.TicketAttachment) bool {
	role := c.GetInt("role")
	if role >= common.RoleAdminUser {
		return true
	}
	userId := c.GetInt("id")
	if userId <= 0 {
		return false
	}
	if a.MessageId == 0 {
		// 未绑定的上传：仅上传者可以访问（用于刚上传时的预览）。
		return a.UserId == userId
	}
	if a.TicketId <= 0 {
		return false
	}
	ticket, err := model.GetUserTicketById(a.TicketId, userId)
	if err != nil || ticket == nil {
		return false
	}
	return true
}

// isPreviewableMime allows only known raster image types, never active documents.
func isPreviewableMime(mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if mime == "" {
		return false
	}
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return true
	}
	return false
}
