import React, { useState, useRef } from 'react';
import {
  Button,
  Card,
  Space,
  TextArea,
  Typography,
  Upload,
} from '@douyinfe/semi-ui';
import { IconUpload } from '@douyinfe/semi-icons';
import { useTicketAttachments } from './useTicketAttachments';

const { Title, Text } = Typography;

// TicketReplyBox 承载工单回复输入框 + 附件上传。
//   - 文本与附件至少二选一才能提交（与后端 AddTicketMessage 约束一致）；
//   - 附件走独立上传接口（/api/ticket/attachment），拿到 id 后再 submit(content, ids)；
//   - 成功发出时清空已选附件，避免下一次回复误带上旧附件。
const TicketReplyBox = ({
  title,
  placeholder,
  submitText,
  disabled = false,
  loading = false,
  quickReplies = [],
  onSubmit,
  t,
}) => {
  const [content, setContent] = useState('');
  const submittingRef = useRef(false);
  const [busy, setBusy] = useState(false);
  const {
    config,
    attachments,
    uploading,
    reset,
    uploadProps,
    uploadRef,
    handlePaste,
  } = useTicketAttachments(t);

  const canReply = !disabled && (content.trim() || attachments.length > 0);

  const handleSubmit = async () => {
    if (!canReply || loading || uploading || submittingRef.current) return;
    submittingRef.current = true;
    setBusy(true);
    try {
      const ids = attachments.map((a) => a.id);
      const ok = await onSubmit?.(content.trim(), ids);
      if (ok) {
        setContent('');
        reset();
      }
    } finally {
      submittingRef.current = false;
      setBusy(false);
    }
  };

  const handleQuickReply = async (phrase) => {
    if (disabled || loading || uploading || submittingRef.current) return;
    submittingRef.current = true;
    setBusy(true);
    const started = Date.now();
    try {
      await onSubmit?.(phrase, []);
    } finally {
      await new Promise((resolve) => setTimeout(resolve, Math.max(0, 2000 - (Date.now() - started))));
      submittingRef.current = false;
      setBusy(false);
    }
  };

  // 粘贴拦截器：把 Ctrl/⌘+V 带过来的文件塞进 Upload 队列。
  // 只拦截 file 类 item；纯文本粘贴不干预 TextArea 默认行为。
  const onPasteCapture = (e) => {
    if (!config.enabled || disabled || loading || busy) return;
    handlePaste(e);
  };

  return (
    <Card className='!rounded-2xl shadow-sm border-0' onPasteCapture={onPasteCapture}>
      <Space vertical align='start' style={{ width: '100%' }} spacing={12}>
        <div>
          <Title heading={5} className='!mb-1'>
            {title || t('回复工单')}
          </Title>
          {disabled && (
            <Text type='tertiary'>{t('当前工单已关闭，如需继续处理请先调整状态')}</Text>
          )}
        </div>
        {quickReplies.length > 0 && (
          <div className='flex flex-wrap gap-2 max-h-48 overflow-y-auto w-full' aria-label={t('Quick replies')}>
            {quickReplies.map((phrase) => (
              <Button key={phrase} disabled={disabled || loading || busy || uploading} style={{ height: 'auto', maxWidth: '100%', whiteSpace: 'normal', overflowWrap: 'anywhere', textAlign: 'left' }} onClick={() => handleQuickReply(phrase)}>{phrase}</Button>
            ))}
          </div>
        )}
        <TextArea
          value={content}
          onChange={setContent}
          autosize={{ minRows: 4, maxRows: 8 }}
          maxLength={5000}
          showClear
          disabled={disabled || loading || busy}
          placeholder={
            placeholder ||
            t('请输入回复内容（支持 Ctrl/⌘+V 粘贴截图或文件）')
          }
        />
        {config.enabled && (
          <Upload {...uploadProps} ref={uploadRef} disabled={disabled || loading || busy}>
            <Button icon={<IconUpload />} disabled={disabled || loading || busy}>
              {t('上传附件')}
            </Button>
            <Text type='tertiary' className='ml-2'>
              {t('最多 {{n}} 个，单个不超过 {{mb}} MB，可粘贴截图', {
                n: config.maxCount,
                mb: Math.floor(config.maxSize / 1024 / 1024),
              })}
            </Text>
          </Upload>
        )}
        <div className='w-full flex justify-end'>
          <Button
            theme='solid'
            type='primary'
            loading={loading || uploading || busy}
            disabled={disabled || !canReply || busy}
            onClick={handleSubmit}
          >
            {submitText || t('发送回复')}
          </Button>
        </div>
      </Space>
    </Card>
  );
};

export default TicketReplyBox;
