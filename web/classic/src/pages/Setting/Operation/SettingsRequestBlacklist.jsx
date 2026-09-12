/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Col,
  Form,
  Radio,
  Row,
  Select,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { IconAlertTriangle } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, showWarning } from '../../../helpers';

const { Text } = Typography;

const emptyRequestBlacklist = {
  mode: 'off',
  enabled_groups: [],
  content_words: '',
  domains: '',
  block_message: '',
  block_status_code: 403,
};

function stringList(value) {
  if (!Array.isArray(value)) return [];
  return value.filter((item) => typeof item === 'string');
}

function listToLines(value) {
  return stringList(value).join('\n');
}

function linesToList(value) {
  return String(value || '')
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseRequestBlacklist(raw) {
  if (!raw || !String(raw).trim()) {
    return { ...emptyRequestBlacklist, enabled_groups: [] };
  }

  try {
    const document = JSON.parse(raw);
    if (!document || typeof document !== 'object' || Array.isArray(document)) {
      return { ...emptyRequestBlacklist, enabled_groups: [] };
    }
    const rawStatus = document.block_status_code;
    return {
      mode: typeof document.mode === 'string' ? document.mode : 'off',
      enabled_groups: stringList(document.enabled_groups),
      content_words: listToLines(document.content_words),
      domains: listToLines(document.domains),
      block_message:
        typeof document.block_message === 'string'
          ? document.block_message
          : '',
      block_status_code:
        typeof rawStatus === 'number' && rawStatus !== 0 ? rawStatus : 403,
    };
  } catch {
    return { ...emptyRequestBlacklist, enabled_groups: [] };
  }
}

function serializeRequestBlacklist(values) {
  return JSON.stringify({
    mode: values.mode,
    enabled_groups: values.enabled_groups,
    content_words: linesToList(values.content_words),
    domains: linesToList(values.domains),
    block_message: values.block_message,
    block_status_code: values.block_status_code,
  });
}

export default function SettingsRequestBlacklist(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [groups, setGroups] = useState([]);
  const [inputs, setInputs] = useState(emptyRequestBlacklist);
  const baselineDocumentRef = useRef(
    serializeRequestBlacklist(emptyRequestBlacklist),
  );
  const refForm = useRef();

  useEffect(() => {
    let active = true;

    // /api/group/ reads an in-memory snapshot and does not query the database.
    async function loadGroups() {
      try {
        const response = await API.get('/api/group/');
        if (!active || !response?.data?.success) return;
        const data = response.data.data;
        setGroups(
          Array.isArray(data) ? data.map(String) : Object.keys(data || {}),
        );
      } catch {
        // The setting remains editable when the optional group list is unavailable.
      }
    }

    void loadGroups();
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    const values = parseRequestBlacklist(props.options?.RequestBlacklist || '');
    baselineDocumentRef.current = serializeRequestBlacklist(values);
    setInputs(values);
    refForm.current?.setValues(values);
  }, [props.options]);

  const setField = (field, value) => {
    setInputs((previous) => ({ ...previous, [field]: value }));
  };

  async function onSubmit() {
    const document = serializeRequestBlacklist(inputs);
    if (document === baselineDocumentRef.current) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }

    setLoading(true);
    try {
      const response = await API.put('/api/option/', {
        key: 'RequestBlacklist',
        value: document,
      });
      if (!response?.data?.success) {
        showError(response?.data?.message || t('保存失败，请重试'));
        return;
      }

      await props.refresh();
      showSuccess(t('保存成功'));
    } catch (error) {
      showError(
        error?.response?.data?.message ||
          error?.message ||
          t('保存失败，请重试'),
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('请求内容与域名黑名单')}>
          <Row gutter={16}>
            <Col xs={24} lg={12}>
              <Form.Slot label={t('模式')}>
                <Radio.Group
                  type='button'
                  value={inputs.mode}
                  onChange={(event) => setField('mode', event.target.value)}
                >
                  <Radio value='off'>{t('关闭')}</Radio>
                  <Radio value='observe'>{t('仅观察')}</Radio>
                  <Radio value='enforce'>{t('拦截')}</Radio>
                </Radio.Group>
                <Text
                  type='tertiary'
                  size='small'
                  style={{ display: 'block', marginTop: 6 }}
                >
                  {inputs.mode === 'off' && t('请求黑名单检查已关闭。')}
                  {inputs.mode === 'observe' &&
                    t('仅观察只记录审计日志并放行请求。')}
                  {inputs.mode === 'enforce' &&
                    t('命中后记录审计日志，并在转发前拦截请求。')}
                </Text>
              </Form.Slot>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24}>
              <Form.Slot label={t('拦截状态码')}>
                <Radio.Group
                  value={String(inputs.block_status_code)}
                  onChange={(event) =>
                    setField('block_status_code', Number(event.target.value))
                  }
                >
                  <div style={{ display: 'grid', gap: 10 }}>
                    <Radio value='403'>
                      <Text strong>{t('403（默认）')}</Text>
                      <Text
                        type='tertiary'
                        size='small'
                        style={{ display: 'block' }}
                      >
                        {t('符合本功能意图：请求被策略拒绝，客户端不应重试。')}
                      </Text>
                    </Radio>
                    <Radio value='400'>
                      <Text strong>400</Text>
                      <Text
                        type='tertiary'
                        size='small'
                        style={{ display: 'block' }}
                      >
                        {t(
                          '表示请求内容不合规；部分 SDK 会当成参数错误直接抛出。',
                        )}
                      </Text>
                    </Radio>
                    <Radio value='503'>
                      <Text strong>
                        <IconAlertTriangle
                          style={{
                            color: 'var(--semi-color-warning)',
                            marginRight: 4,
                            verticalAlign: 'text-bottom',
                          }}
                        />
                        503
                      </Text>
                      <Text
                        type='warning'
                        size='small'
                        style={{ display: 'block' }}
                      >
                        {t(
                          '客户端重试机制会生效，同一份黑名单内容会被反复提交并白白消耗算力。',
                        )}
                      </Text>
                    </Radio>
                  </div>
                </Radio.Group>
              </Form.Slot>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24} lg={12}>
              <Form.Slot label={t('生效分组')}>
                <Select
                  multiple
                  filter
                  style={{ width: '100%' }}
                  placeholder={t('全部分组')}
                  value={inputs.enabled_groups}
                  onChange={(value) =>
                    setField(
                      'enabled_groups',
                      Array.isArray(value) ? value : [],
                    )
                  }
                >
                  {groups.map((group) => (
                    <Select.Option key={group} value={group}>
                      {group}
                    </Select.Option>
                  ))}
                </Select>
                <Text
                  type='tertiary'
                  size='small'
                  style={{ display: 'block', marginTop: 6 }}
                >
                  {t('留空表示对全部分组生效。')}
                </Text>
              </Form.Slot>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24} lg={12}>
              <Form.TextArea
                field='content_words'
                label={t('内容关键词')}
                extraText={t('一行一个内容关键词。')}
                placeholder={t('一行一个内容关键词')}
                autosize={{ minRows: 8, maxRows: 16 }}
                style={{ fontFamily: 'JetBrains Mono, Consolas' }}
                onChange={(value) => setField('content_words', value)}
              />
            </Col>
            <Col xs={24} lg={12}>
              <Form.TextArea
                field='domains'
                label={t('域名')}
                extraText={t(
                  '一行一个主机名，不带 http://、路径或通配符。example.com 同时匹配自身及任意子域，但不匹配 notexample.com 或 example.com.evil.com。',
                )}
                placeholder='example.com'
                autosize={{ minRows: 8, maxRows: 16 }}
                style={{ fontFamily: 'JetBrains Mono, Consolas' }}
                onChange={(value) => setField('domains', value)}
              />
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24}>
              <Form.Input
                field='block_message'
                label={t('拦截文案')}
                extraText={t(
                  '该文案会连同 request id 一起返回给调用方。不要填写域名、URL、IP 或密钥，否则保存会被拒绝。',
                )}
                onChange={(value) => setField('block_message', value)}
              />
            </Col>
          </Row>

          <Banner
            type='warning'
            title={t('能力边界')}
            description={
              <ul style={{ margin: 0, paddingLeft: 20 }}>
                <li>
                  {t(
                    '只扫描请求文本。域名黑名单只在域名以文本形式出现在 prompt 中时生效。',
                  )}
                </li>
                <li>
                  {t(
                    '不覆盖 image_url、file、video_url 等结构化 URL 字段，Midjourney 与视频或音乐任务的 prompt、/v1/alpha/search、Realtime 语音会话、音频转写 prompt、上传文件内容及 base64。',
                  )}
                </li>
                <li>
                  {t(
                    '无法拦截短链、跳转、编码混淆、OCR，以及上游模型自主生成的目标。',
                  )}
                </li>
                <li>
                  {t(
                    'Unicode 域名需填写 punycode。例え.jp 会保存为 xn--r8jz45g.jp；prompt 中的原文 例え.jp 不会命中，punycode 才会命中。',
                  )}
                </li>
                <li>
                  {t(
                    '真正的出站访问授权应在实际发起 HTTP 请求的 egress 或 tool 层完成；本功能不是完整的渗透防护。',
                  )}
                </li>
              </ul>
            }
            style={{ marginBottom: 16 }}
          />

          <Button size='default' onClick={onSubmit}>
            {t('保存请求黑名单设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
