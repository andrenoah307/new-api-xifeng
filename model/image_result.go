package model

import (
	"github.com/QuantumNous/new-api/common"
)

// ImageResult 记录一次成功生图请求落地保存的结果。
// 图片二进制不入库：走 service/attachment 存储抽象写本地磁盘，
// Files 仅保存文件引用（JSON 数组），由清理任务按保留天数删除文件与本记录。
type ImageResult struct {
	Id          int    `json:"id" gorm:"primaryKey"`
	UserId      int    `json:"user_id" gorm:"index"`
	TokenId     int    `json:"token_id"`
	ModelName   string `json:"model_name" gorm:"type:varchar(255)"`
	Mode        string `json:"mode" gorm:"type:varchar(20)"` // generations / edits
	Prompt      string `json:"prompt" gorm:"type:text"`      // 入库前截断，避免超长 prompt 撑爆行
	RequestId   string `json:"request_id" gorm:"type:varchar(64);index"`
	Files       string `json:"files" gorm:"type:text"` // JSON: []ImageResultFile
	CreatedTime int64  `json:"created_time" gorm:"index"`
}

// ImageResultFile 是 Files 字段里单张图片的引用信息。
// Key 是存储层内部路径，不直接返回给用户（下载走鉴权接口按序号取）。
type ImageResultFile struct {
	Key    string `json:"key"`
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
	Source string `json:"source"` // b64 = 上游返回 base64；url = 上游返回链接后服务端下载
}

func (r *ImageResult) Insert() error {
	return DB.Create(r).Error
}

func (r *ImageResult) GetFiles() []ImageResultFile {
	var files []ImageResultFile
	if r.Files == "" {
		return files
	}
	if err := common.UnmarshalJsonStr(r.Files, &files); err != nil {
		return nil
	}
	return files
}

func (r *ImageResult) SetFiles(files []ImageResultFile) error {
	data, err := common.Marshal(files)
	if err != nil {
		return err
	}
	r.Files = string(data)
	return nil
}

func GetImageResultById(id int) (*ImageResult, error) {
	var result ImageResult
	if err := DB.First(&result, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

// GetUserImageResults 分页返回某用户的生图记录，按时间倒序。
func GetUserImageResults(userId int, startIdx int, num int) ([]*ImageResult, int64, error) {
	var results []*ImageResult
	var total int64
	query := DB.Model(&ImageResult{}).Where("user_id = ?", userId)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&results).Error; err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// GetAllImageResults 管理端分页，userId=0 表示不过滤用户。
func GetAllImageResults(userId int, startIdx int, num int) ([]*ImageResult, int64, error) {
	var results []*ImageResult
	var total int64
	query := DB.Model(&ImageResult{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&results).Error; err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// GetExpiredImageResults 返回创建时间早于 cutoff 的记录（清理任务用）。
func GetExpiredImageResults(cutoff int64, limit int) ([]*ImageResult, error) {
	var results []*ImageResult
	err := DB.Where("created_time < ?", cutoff).Order("id asc").Limit(limit).Find(&results).Error
	return results, err
}

func DeleteImageResultById(id int) error {
	return DB.Delete(&ImageResult{}, "id = ?", id).Error
}
