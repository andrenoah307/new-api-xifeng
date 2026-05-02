import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Card,
  Typography,
  Select,
  Button,
  Input,
  TextArea,
  Space,
  Tag,
  Tooltip,
  Spin,
  Banner,
  Popconfirm,
  Modal,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const { Text, Title, Paragraph } = Typography;

/**
 * 邮件模板编辑 + 预览
 *
 * 后端接口：
 *   GET  /api/option/email_templates
 *   POST /api/option/email_templates/preview   { key, subject, body }
 *   POST /api/option/email_templates/reset     { key }
 *   PUT  /api/option/                          { key, value }  (用来保存单项)
 */
const EmailTemplateSetting = () => {
  const { t } = useTranslation();

  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [templates, setTemplates] = useState([]);
  const [activeKey, setActiveKey] = useState('');
  const [drafts, setDrafts] = useState({});
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewSubject, setPreviewSubject] = useState('');
  const [previewBody, setPreviewBody] = useState('');

  const subjectRef = useRef(null);
  const bodyRef = useRef(null);
  const lastFocusedRef = useRef('body');

  const activeTemplate = useMemo(
    () => templates.find((x) => x.key === activeKey),
    [templates, activeKey],
  );
  const currentDraft = drafts[activeKey] || { subject: '', body: '' };

  const fetchTemplates = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/email_templates');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setTemplates(data || []);
      if ((data || []).length > 0) {
        const initial = {};
        data.forEach((tpl) => {
          initial[tpl.key] = {
            subject: tpl.current_subject || '',
            body: tpl.current_body || '',
          };
        });
        setDrafts(initial);
        setActiveKey((prev) => prev || data[0].key);
      }
    } catch (e) {
      showError(t('加载邮件模板失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTemplates();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const updateDraft = (patch) => {
    setDrafts((prev) => ({
      ...prev,
      [activeKey]: { ...(prev[activeKey] || {}), ...patch },
    }));
  };

  const insertVariable = (name) => {
    const token = `{{${name}}}`;
    const target = lastFocusedRef.current === 'subject' ? subjectRef : bodyRef;
    // Semi Input/TextArea 暴露的原生 DOM 在 .inputRef 或 .textareaRef 上，回退到直接追加
    const el =
      target?.current?.inputRef?.current ||
      target?.current?.textareaRef?.current ||
      null;
    if (el && typeof el.selectionStart === 'number') {
      const start = el.selectionStart;
      const end = el.selectionEnd;
      const current =
        lastFocusedRef.current === 'subject'
          ? currentDraft.subject
          : currentDraft.body;
      const next =
        (current || '').slice(0, start) + token + (current || '').slice(end);
      updateDraft(
        lastFocusedRef.current === 'subject'
          ? { subject: next }
          : { body: next },
      );
      requestAnimationFrame(() => {
        try {
          el.focus();
          const pos = start + token.length;
          el.setSelectionRange(pos, pos);
        } catch (_) {
          /* noop */
        }
      });
    } else {
      if (lastFocusedRef.current === 'subject') {
        updateDraft({ subject: (currentDraft.subject || '') + token });
      } else {
        updateDraft({ body: (currentDraft.body || '') + token });
      }
    }
  };

  const handlePreview = async () => {
    if (!activeKey) return;
    setPreviewing(true);
    try {
      const res = await API.post('/api/option/email_templates/preview', {
        key: activeKey,
        subject: currentDraft.subject,
        body: currentDraft.body,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setPreviewSubject(data.subject || '');
      setPreviewBody(data.body || '');
      setPreviewOpen(true);
    } catch (e) {
      showError(t('预览失败'));
    } finally {
      setPreviewing(false);
    }
  };

  const handleSave = async () => {
    if (!activeTemplate) return;
    setSaving(true);
    try {
      const subjectKey = `EmailTemplate.${activeKey}.subject`;
      const bodyKey = `EmailTemplate.${activeKey}.body`;
      // 与默认值相同时存空串，让后端读取时回落到默认
      const subjectValue =
        currentDraft.subject === activeTemplate.default_subject
          ? ''
          : currentDraft.subject || '';
      const bodyValue =
        currentDraft.body === activeTemplate.default_body
          ? ''
          : currentDraft.body || '';

      const [r1, r2] = await Promise.all([
        API.put('/api/option/', { key: subjectKey, value: subjectValue }),
        API.put('/api/option/', { key: bodyKey, value: bodyValue }),
      ]);
      if (!r1.data.success) {
        showError(r1.data.message);
        return;
      }
      if (!r2.data.success) {
        showError(r2.data.message);
        return;
      }
      showSuccess(t('已保存'));
      await fetchTemplates();
    } catch (e) {
      showError(t('保存失败'));
    } finally {
      setSaving(false);
    }
  };

  const handleReset = async () => {
    if (!activeTemplate) return;
    try {
      const res = await API.post('/api/option/email_templates/reset', {
        key: activeKey,
      });
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('已恢复默认'));
      await fetchTemplates();
    } catch (e) {
      showError(t('操作失败'));
    }
  };

  return (
    <Card>
      <Title heading={5} style={{ marginBottom: 4 }}>
        {t('邮件模板编辑')}
      </Title>
      <Paragraph type='tertiary' style={{ marginBottom: 16, fontSize: 13 }}>
        {t(
          '使用 {{变量名}} 作为占位，系统发送时会自动替换。模板正文支持完整 HTML；若保留为默认值则使用系统内置样式。',
        )}
      </Paragraph>

      <Spin spinning={loading}>
        {templates.length > 0 && (
          <>
            <div style={{ marginBottom: 12 }}>
              <Text strong style={{ marginRight: 12 }}>
                {t('选择模板')}
              </Text>
              <Select
                style={{ width: 320 }}
                value={activeKey}
                onChange={(v) => setActiveKey(v)}
                optionList={templates.map((tpl) => ({
                  label: tpl.customized
                    ? `${tpl.name}（${t('已自定义')}）`
                    : tpl.name,
                  value: tpl.key,
                }))}
              />
              {activeTemplate?.customized && (
                <Tag style={{ marginLeft: 8 }} color='blue'>
                  {t('已自定义')}
                </Tag>
              )}
            </div>

            {activeTemplate && (
              <>
                <Banner
                  type='info'
                  closeIcon={null}
                  description={activeTemplate.description}
                  style={{ marginBottom: 12 }}
                />

                <div style={{ marginBottom: 8 }}>
                  <Text strong style={{ fontSize: 13 }}>
                    {t('可用变量')}
                  </Text>
                  <Paragraph
                    type='tertiary'
                    style={{ margin: '4px 0 8px', fontSize: 12 }}
                  >
                    {t('点击标签即可插入到当前光标位置')}
                  </Paragraph>
                  <Space wrap>
                    {(activeTemplate.variables || []).map((v) => (
                      <Tooltip
                        key={v.name}
                        content={v.description || v.name}
                        position='top'
                      >
                        <Tag
                          color='light-blue'
                          style={{ cursor: 'pointer' }}
                          onClick={() => insertVariable(v.name)}
                        >
                          {`{{${v.name}}}`}
                        </Tag>
                      </Tooltip>
                    ))}
                  </Space>
                </div>

                <div style={{ marginTop: 12 }}>
                  <Text strong style={{ fontSize: 13 }}>
                    {t('邮件主题')}
                  </Text>
                  <Input
                    ref={subjectRef}
                    value={currentDraft.subject}
                    onChange={(v) => updateDraft({ subject: v })}
                    onFocus={() => (lastFocusedRef.current = 'subject')}
                    placeholder={activeTemplate.default_subject}
                    style={{ marginTop: 6 }}
                  />
                </div>

                <div style={{ marginTop: 12 }}>
                  <Text strong style={{ fontSize: 13 }}>
                    {t('邮件正文（HTML）')}
                  </Text>
                  <TextArea
                    ref={bodyRef}
                    value={currentDraft.body}
                    onChange={(v) => updateDraft({ body: v })}
                    onFocus={() => (lastFocusedRef.current = 'body')}
                    placeholder={activeTemplate.default_body}
                    rows={16}
                    style={{
                      marginTop: 6,
                      fontFamily:
                        'Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
                      fontSize: 12,
                    }}
                  />
                </div>

                <div style={{ marginTop: 16 }}>
                  <Space>
                    <Button
                      type='primary'
                      theme='solid'
                      loading={saving}
                      onClick={handleSave}
                    >
                      {t('保存模板')}
                    </Button>
                    <Button loading={previewing} onClick={handlePreview}>
                      {t('预览')}
                    </Button>
                    <Popconfirm
                      title={t('确定恢复为默认模板？')}
                      content={t('自定义的主题与正文都会被清空')}
                      onConfirm={handleReset}
                    >
                      <Button type='danger' theme='borderless'>
                        {t('恢复默认')}
                      </Button>
                    </Popconfirm>
                  </Space>
                </div>
              </>
            )}
          </>
        )}
      </Spin>

      <EmailTemplatePreviewDialog
        visible={previewOpen}
        subject={previewSubject}
        body={previewBody}
        onClose={() => setPreviewOpen(false)}
      />
    </Card>
  );
};

/** 独立出来避免父组件重渲染时 iframe 重建 */
const EmailTemplatePreviewDialog = ({ visible, subject, body, onClose }) => {
  const { t } = useTranslation();
  return (
    <Modal
      title={t('邮件预览')}
      visible={visible}
      onCancel={onClose}
      onOk={onClose}
      centered
      width={820}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
      footer={<Button onClick={onClose}>{t('关闭')}</Button>}
    >
      <div style={{ marginBottom: 10 }}>
        <Text type='tertiary' style={{ fontSize: 12 }}>
          {t('主题')}
        </Text>
        <div
          style={{
            marginTop: 4,
            padding: '8px 12px',
            background: '#f7f8fa',
            borderRadius: 4,
            fontSize: 14,
          }}
        >
          {subject || '(empty)'}
        </div>
      </div>
      <div style={{ marginTop: 12 }}>
        <Text type='tertiary' style={{ fontSize: 12 }}>
          {t('正文预览')}
        </Text>
        <iframe
          title='email-preview'
          srcDoc={body || ''}
          sandbox=''
          style={{
            marginTop: 4,
            width: '100%',
            height: 520,
            border: '1px solid #eee',
            borderRadius: 4,
            background: '#fff',
          }}
        />
      </div>
    </Modal>
  );
};

export default EmailTemplateSetting;
