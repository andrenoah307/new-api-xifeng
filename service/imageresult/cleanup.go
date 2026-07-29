package imageresult

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const cleanupBatch = 500

// CleanupExpiredOnce 删除超过保留天数的生图记录：先删存储文件再删行，
// 文件删失败则保留该行（下轮重试），保证不产生找不回的孤儿文件。
// 由系统任务框架调度（DB 租约多实例去重），返回本轮清理摘要。
func CleanupExpiredOnce(ctx context.Context) (string, error) {
	retentionDays := operation_setting.ImageResultRetentionDays
	if retentionDays < 0 {
		retentionDays = 0
	}
	cutoff := common.GetTimestamp() - int64(retentionDays)*86400

	expired, err := model.GetExpiredImageResults(cutoff, cleanupBatch)
	if err != nil {
		return "", fmt.Errorf("list expired image results: %w", err)
	}
	if len(expired) == 0 {
		return "no expired image results", nil
	}

	store, err := Store()
	if err != nil {
		return "", fmt.Errorf("init storage: %w", err)
	}

	removed, failed := 0, 0
	for _, r := range expired {
		fileFailed := false
		for _, f := range r.GetFiles() {
			// 存储实现对 NotFound 幂等（不报错），重复删除安全。
			if err := store.Delete(ctx, f.Key); err != nil {
				common.SysError(fmt.Sprintf("image result cleanup: delete file %s failed: %s", f.Key, err.Error()))
				fileFailed = true
			}
		}
		if fileFailed {
			failed++
			continue
		}
		if err := model.DeleteImageResultById(r.Id); err != nil {
			common.SysError(fmt.Sprintf("image result cleanup: delete record %d failed: %s", r.Id, err.Error()))
			failed++
			continue
		}
		removed++
	}
	return fmt.Sprintf("removed %d expired image result(s), %d failed", removed, failed), nil
}
