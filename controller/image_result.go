package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/attachment"
	"github.com/QuantumNous/new-api/service/imageresult"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 生图结果查看/下载端点。
// 存储 key 不下发给前端：列表只给每张图的序号 + 元信息，
// 下载按 记录 id + 序号 取，归属校验后由后端流式返回。

type imageResultFileView struct {
	Idx    int    `json:"idx"`
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
	Source string `json:"source"`
}

type imageResultView struct {
	Id          int                   `json:"id"`
	UserId      int                   `json:"user_id"`
	Username    string                `json:"username"`
	ModelName   string                `json:"model_name"`
	Mode        string                `json:"mode"`
	Prompt      string                `json:"prompt"`
	RequestId   string                `json:"request_id"`
	CreatedTime int64                 `json:"created_time"`
	Files       []imageResultFileView `json:"files"`
}

func toImageResultView(r *model.ImageResult) imageResultView {
	files := r.GetFiles()
	views := make([]imageResultFileView, 0, len(files))
	for i, f := range files {
		views = append(views, imageResultFileView{Idx: i, Mime: f.Mime, Size: f.Size, Source: f.Source})
	}
	// 用户名走缓存查询（Redis 优先），查不到留空即可，不影响列表
	username, _ := model.GetUsernameById(r.UserId, false)
	return imageResultView{
		Id:          r.Id,
		UserId:      r.UserId,
		Username:    username,
		ModelName:   r.ModelName,
		Mode:        r.Mode,
		Prompt:      r.Prompt,
		RequestId:   r.RequestId,
		CreatedTime: r.CreatedTime,
		Files:       views,
	}
}

// GetUserImageResults 分页返回当前用户的生图记录。
func GetUserImageResults(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	results, total, err := model.GetUserImageResults(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views := make([]imageResultView, 0, len(results))
	for _, r := range results {
		views = append(views, toImageResultView(r))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(views)
	common.ApiSuccess(c, pageInfo)
}

// GetAllImageResults 管理端分页，支持 user_id 过滤。
func GetAllImageResults(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId, _ := strconv.Atoi(c.Query("user_id"))
	results, total, err := model.GetAllImageResults(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views := make([]imageResultView, 0, len(results))
	for _, r := range results {
		views = append(views, toImageResultView(r))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(views)
	common.ApiSuccess(c, pageInfo)
}

// DownloadUserImageResultFile 用户下载自己记录里的某张图（归属校验）。
func DownloadUserImageResultFile(c *gin.Context) {
	streamImageResultFile(c, true)
}

// DownloadImageResultFile 管理员下载任意记录里的某张图。
func DownloadImageResultFile(c *gin.Context) {
	streamImageResultFile(c, false)
}

func streamImageResultFile(c *gin.Context, checkOwnership bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	idx, err := strconv.Atoi(c.Param("idx"))
	if err != nil || idx < 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	record, err := model.GetImageResultById(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "image result not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	if checkOwnership && record.UserId != c.GetInt("id") {
		common.ApiErrorI18n(c, i18n.MsgForbidden)
		return
	}

	files := record.GetFiles()
	if idx >= len(files) {
		common.ApiErrorMsg(c, "image not found")
		return
	}
	file := files[idx]

	store, err := imageresult.Store()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	reader, err := store.Get(c.Request.Context(), file.Key)
	if err != nil {
		if errors.Is(err, attachment.ErrNotFound) {
			common.ApiErrorMsg(c, "image content missing")
			return
		}
		common.ApiError(c, err)
		return
	}
	defer reader.Close()

	c.Writer.Header().Set("Content-Type", file.Mime)
	c.Writer.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
	// 生图结果创建后不可变；带鉴权头请求，缓存标 private。
	c.Writer.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, reader); err != nil {
		common.SysLog("image result download: copy failed: " + err.Error())
	}
}
