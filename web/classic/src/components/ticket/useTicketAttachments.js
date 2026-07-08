/*
 * useTicketAttachments
 *
 * 统一工单附件的客户端校验 / 上传 / 撤回逻辑。
 * 创建工单 Modal 和工单详情页的回复框都复用这一份实现，避免两处校验漂移。
 *
 * 返回值：
 *   config      — 从 /api/status 下发的限制（开关、大小、数量、扩展名），带兜底默认值
 *   attachments — 已成功上传、等待提交的附件列表 {id, uid, file_name, mime_type, size, previewable}
 *   uploading   — 是否有上传中的请求
 *   uploadProps — 可直接摊到 <Upload {...uploadProps}> 上的 props（beforeUpload/customRequest/onRemove/accept/limit）
 *   reset       — 清空已选附件（不触发后端删除；清理交给后端孤儿任务）
 *   discardAll  — 主动撤销所有未提交附件（用户点取消时调用，释放存储空间）
 */

import { useCallback, useContext, useMemo, useRef, useState } from 'react';
import { Toast } from '@douyinfe/semi-ui';
import { StatusContext } from '../../context/Status';
import { API } from '../../helpers';

// 把常见 MIME 映射回扩展名，给 clipboard 里未命名的文件兜底。
const MIME_TO_EXT = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/jpg': 'jpg',
  'image/gif': 'gif',
  'image/webp': 'webp',
  'image/bmp': 'bmp',
  'application/pdf': 'pdf',
  'application/json': 'json',
  'application/xml': 'xml',
  'text/plain': 'txt',
  'text/markdown': 'md',
  'text/csv': 'csv',
};

function extFromMime(mime) {
  if (!mime) return '';
  const m = String(mime).toLowerCase().split(';')[0].trim();
  return MIME_TO_EXT[m] || '';
}

// Clipboard 里的截图通常叫 "image.png"（Chrome）或没有 name（Safari）。
// 统一补一个时间戳文件名，避免后端看到重名或空名。
function ensureFileName(file) {
  const origName = (file.name || '').trim();
  if (origName && origName !== 'image.png' && origName !== 'blob') {
    return origName;
  }
  const ext = extFromMime(file.type) || 'bin';
  const ts = new Date()
    .toISOString()
    .replace(/[-:]/g, '')
    .replace(/\..+$/, '');
  return `pasted-${ts}.${ext}`;
}

const DEFAULT_EXTS =
  'jpg,jpeg,png,gif,webp,bmp,json,xml,txt,log,md,csv,pdf';
const DEFAULT_MAX_SIZE = 50 * 1024 * 1024;
const DEFAULT_MAX_COUNT = 5;

export function useTicketAttachments(t) {
  const [statusState] = useContext(StatusContext);
  const [attachments, setAttachments] = useState([]);
  const [uploading, setUploading] = useState(false);
  const uploadRef = useRef(null);

  const config = useMemo(() => {
    const status = statusState?.status || {};
    const hasFlag = Object.prototype.hasOwnProperty.call(
      status,
      'ticket_attachment_enabled',
    );
    const enabled = hasFlag ? Boolean(status.ticket_attachment_enabled) : true;
    const maxSize = Number(status.ticket_attachment_max_size) || DEFAULT_MAX_SIZE;
    const maxCount = Number(status.ticket_attachment_max_count) || DEFAULT_MAX_COUNT;
    const exts = String(status.ticket_attachment_allowed_exts || DEFAULT_EXTS)
      .split(',')
      .map((s) => s.trim().toLowerCase())
      .filter(Boolean);
    return {
      enabled,
      maxSize,
      maxCount,
      exts,
      accept: exts.map((e) => '.' + e).join(','),
    };
  }, [statusState]);

  const beforeUpload = ({ file }) => {
    if (!config.enabled) {
      Toast.warning(t('附件功能未启用'));
      return false;
    }
    if (attachments.length >= config.maxCount) {
      Toast.warning(t('最多可上传 {{n}} 个附件', { n: config.maxCount }));
      return false;
    }
    if (file.size > config.maxSize) {
      Toast.warning(
        t('单个附件不能超过 {{mb}} MB', {
          mb: Math.floor(config.maxSize / 1024 / 1024),
        }),
      );
      return false;
    }
    const fileName = String(file.name || '');
    const dot = fileName.lastIndexOf('.');
    const ext = dot >= 0 ? fileName.slice(dot + 1).toLowerCase() : '';
    if (ext === 'svg') {
      Toast.warning(t('出于安全考虑，SVG 附件被禁用'));
      return false;
    }
    if (config.exts.length && !config.exts.includes(ext)) {
      Toast.warning(t('不支持的附件类型'));
      return false;
    }
    return true;
  };

  const customRequest = async ({ file, fileInstance, onSuccess, onError }) => {
    const form = new FormData();
    form.append('file', fileInstance || file);
    setUploading(true);
    try {
      const res = await API.post('/api/ticket/attachment', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      if (!res.data?.success) {
        const msg = res.data?.message || t('附件上传失败');
        Toast.error(msg);
        onError?.({ status: 0, message: msg });
        return;
      }
      const data = res.data.data;
      setAttachments((prev) => [...prev, { ...data, uid: file.uid }]);
      onSuccess?.(data);
    } catch (err) {
      const msg = err?.message || t('附件上传失败');
      Toast.error(msg);
      onError?.({ status: 0, message: msg });
    } finally {
      setUploading(false);
    }
  };

  const handleRemove = async (file) => {
    const target = attachments.find((a) => a.uid === file.uid);
    if (target?.id) {
      try {
        await API.delete(`/api/ticket/attachment/${target.id}`);
      } catch (err) {
        // 忽略：后端清理任务会兜底
      }
    }
    setAttachments((prev) => prev.filter((a) => a.uid !== file.uid));
    return true;
  };

  // handlePaste 从 clipboard 里收集 File 对象并塞进 Upload 的上传队列。
  //   - 只处理文件类（items[i].kind === 'file'）；纯文本粘贴不拦截，保留 TextArea 默认行为；
  //   - 从 MIME 推断合适的文件名（截图通常没名字）；
  //   - 通过 uploadRef.insert() 走常规 beforeUpload + customRequest，校验和上传复用同一套逻辑。
  const handlePaste = useCallback(
    (event) => {
      const clipboardData = event?.clipboardData || window.clipboardData;
      if (!clipboardData) return;
      const items = Array.from(clipboardData.items || []);
      const files = [];
      for (const item of items) {
        if (item.kind !== 'file') continue;
        const f = item.getAsFile();
        if (!f) continue;
        files.push(f);
      }
      if (files.length === 0) return;
      // 阻止默认粘贴（避免把图片路径/文件名以文本形式塞进 TextArea）。
      event.preventDefault();

      if (!uploadRef.current || typeof uploadRef.current.insert !== 'function') {
        Toast.warning(t('当前页面暂不支持粘贴附件'));
        return;
      }
      const prepared = files.map((f) => {
        const name = ensureFileName(f);
        // File 是不可变的，但 Semi Upload 只用 name/type/size + 原对象，
        // 所以这里新建一个 File 保留二进制内容、替换 name 即可。
        try {
          return new File([f], name, { type: f.type, lastModified: f.lastModified });
        } catch (_) {
          // 部分老浏览器不允许 new File()，退回原对象并尝试改 name（非严格）。
          try {
            Object.defineProperty(f, 'name', { value: name, configurable: true });
          } catch (e) {}
          return f;
        }
      });
      try {
        uploadRef.current.insert(prepared);
      } catch (err) {
        Toast.error(t('粘贴附件失败'));
      }
    },
    [t],
  );

  // 对外仅提供 reset（清前端状态），不主动串行删后端。
  // 主动删除全部（用户取消创建时）由调用方决定是否调用 discardAll。
  const reset = () => setAttachments([]);

  const discardAll = async () => {
    const ids = attachments.map((a) => a.id).filter(Boolean);
    setAttachments([]);
    for (const id of ids) {
      try {
        await API.delete(`/api/ticket/attachment/${id}`);
      } catch (err) {
        // 忽略
      }
    }
  };

  return {
    config,
    attachments,
    uploading,
    reset,
    discardAll,
    handlePaste,
    uploadRef,
    uploadProps: {
      multiple: true,
      draggable: false,
      accept: config.accept,
      beforeUpload,
      customRequest,
      onRemove: handleRemove,
      limit: config.maxCount,
      showReplace: false,
      showUploadList: true,
    },
  };
}
