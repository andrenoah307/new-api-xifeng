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

import React, { useEffect, useState, useRef } from 'react';
import {
  Button,
  Col,
  Form,
  Row,
  Spin,
  DatePicker,
  Typography,
  Modal,
  Table,
  Tag,
  Banner,
} from '@douyinfe/semi-ui';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';

const { Text } = Typography;

const IP_MODES = ['auto', 'trusted_header', 'xff', 'remote_addr'];

const PRESET_IP_HEADERS = [
  'CF-Connecting-IP',
  'X-Real-IP',
  'True-Client-IP',
  'X-Forwarded-For',
];

// "" 表示管理员从未显式选择过判定方式，按旧的「信任上游 IP 头」开关推导
function effectiveIpMode(ipMode, trustedIpHeaderEnabled) {
  if (IP_MODES.includes(ipMode)) return ipMode;
  return String(trustedIpHeaderEnabled) === 'true' ? 'trusted_header' : 'auto';
}

export default function SettingsLog(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [loadingCleanHistoryLog, setLoadingCleanHistoryLog] = useState(false);
  const [inputs, setInputs] = useState({
    LogConsumeEnabled: false,
    ForceRecordIPEnabled: false,
    'risk_control.ip_mode': 'auto',
    'risk_control.trusted_ip_header': 'X-Real-IP',
    'risk_control.xff_index': '-1',
    historyTimestamp: dayjs().subtract(1, 'month').toDate(),
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const [showIpDetectModal, setShowIpDetectModal] = useState(false);
  const [ipDiagnosis, setIpDiagnosis] = useState(null);
  const [ipDetectLoading, setIpDetectLoading] = useState(false);

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow).filter(
      (item) => item.key !== 'historyTimestamp',
    );

    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else {
        value = inputs[item.key];
      }
      return API.put('/api/option/', {
        key: item.key,
        value,
      });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }
  async function onCleanHistoryLog() {
    if (!inputs.historyTimestamp) {
      showError(t('请选择日志记录时间'));
      return;
    }

    const now = dayjs();
    const targetDate = dayjs(inputs.historyTimestamp);
    const targetTime = targetDate.format('YYYY-MM-DD HH:mm:ss');
    const currentTime = now.format('YYYY-MM-DD HH:mm:ss');
    const daysDiff = now.diff(targetDate, 'day');

    Modal.confirm({
      title: t('确认清除历史日志'),
      content: (
        <div style={{ lineHeight: '1.8' }}>
          <p>
            <Text>{t('当前时间')}：</Text>
            <Text strong style={{ color: '#52c41a' }}>
              {currentTime}
            </Text>
          </p>
          <p>
            <Text>{t('选择时间')}：</Text>
            <Text strong type='danger'>
              {targetTime}
            </Text>
            {daysDiff > 0 && (
              <Text type='tertiary'>
                {' '}
                ({t('约')} {daysDiff} {t('天前')})
              </Text>
            )}
          </p>
          <div
            style={{
              background: '#fff7e6',
              border: '1px solid #ffd591',
              padding: '12px',
              borderRadius: '4px',
              marginTop: '12px',
              color: '#333',
            }}
          >
            <Text strong style={{ color: '#d46b08' }}>
              ⚠️ {t('注意')}：
            </Text>
            <Text style={{ color: '#333' }}>{t('将删除')} </Text>
            <Text strong style={{ color: '#cf1322' }}>
              {targetTime}
            </Text>
            {daysDiff > 0 && (
              <Text style={{ color: '#8c8c8c' }}>
                {' '}
                ({t('约')} {daysDiff} {t('天前')})
              </Text>
            )}
            <Text style={{ color: '#333' }}> {t('之前的所有日志')}</Text>
          </div>
          <p style={{ marginTop: '12px' }}>
            <Text type='danger'>
              {t('此操作不可恢复，请仔细确认时间后再操作！')}
            </Text>
          </p>
        </div>
      ),
      okText: t('确认删除'),
      cancelText: t('取消'),
      okType: 'danger',
      onOk: async () => {
        try {
          setLoadingCleanHistoryLog(true);
          const res = await API.delete(
            `/api/log/?target_timestamp=${Date.parse(inputs.historyTimestamp) / 1000}`,
          );
          const { success, message, data } = res.data;
          if (success) {
            showSuccess(`${data} ${t('条日志已清理！')}`);
            return;
          } else {
            throw new Error(t('日志清理失败：') + message);
          }
        } catch (error) {
          showError(error.message);
        } finally {
          setLoadingCleanHistoryLog(false);
        }
      },
    });
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    // ip_mode 为空时归一为推导值并同步写入快照，避免出现「未修改却提示有变更」
    currentInputs['risk_control.ip_mode'] = effectiveIpMode(
      currentInputs['risk_control.ip_mode'],
      props.options['risk_control.trusted_ip_header_enabled'],
    );
    if (!currentInputs['risk_control.trusted_ip_header']) {
      currentInputs['risk_control.trusted_ip_header'] = 'X-Real-IP';
    }
    if (
      currentInputs['risk_control.xff_index'] === undefined ||
      currentInputs['risk_control.xff_index'] === ''
    ) {
      currentInputs['risk_control.xff_index'] = '-1';
    } else {
      currentInputs['risk_control.xff_index'] = String(
        currentInputs['risk_control.xff_index'],
      );
    }
    currentInputs['historyTimestamp'] = inputs.historyTimestamp;
    setInputs({ ...inputs, ...currentInputs });
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  // 请求头名称 / XFF 位置 是随判定方式切换而挂载/卸载的条件字段。
  // 加载数据时 setValues 无法写入尚未挂载的字段，导致刷新后这两项显示为空
  // （inputs 里其实有正确值）。这里在方式切换、字段挂载完成后从 inputs 回填一次显示值。
  useEffect(() => {
    if (!refForm.current) return;
    const mode = inputs['risk_control.ip_mode'];
    if (mode === 'trusted_header') {
      refForm.current.setValue(
        'risk_control.trusted_ip_header',
        inputs['risk_control.trusted_ip_header'],
      );
    } else if (mode === 'xff') {
      refForm.current.setValue(
        'risk_control.xff_index',
        inputs['risk_control.xff_index'],
      );
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [inputs['risk_control.ip_mode']]);

  async function fetchIpDiagnosis() {
    setIpDetectLoading(true);
    try {
      const res = await API.get('/api/risk/detect-ip');
      const { success, message, data } = res.data;
      if (success) {
        setIpDiagnosis(data);
      } else {
        showError(message || t('检测失败'));
      }
    } catch (error) {
      showError(t('检测失败'));
    } finally {
      setIpDetectLoading(false);
    }
  }

  function openIpDetectModal() {
    setShowIpDetectModal(true);
    fetchIpDiagnosis();
  }

  // 按当前编辑中的头名/XFF位置，从诊断快照本地计算各方式解析出的 IP
  function previewIpForMode(mode) {
    if (!ipDiagnosis) return '';
    const items = ipDiagnosis.items || [];
    const remoteItem = items.find((item) => item.source === 'remote_addr');
    const remoteIp = remoteItem ? remoteItem.parsed_ip : '';
    if (mode === 'remote_addr') return remoteIp;
    if (mode === 'trusted_header') {
      const header = String(inputs['risk_control.trusted_ip_header'] || '')
        .trim()
        .toLowerCase();
      const item = items.find(
        (candidate) =>
          candidate.source === 'header' &&
          candidate.name.toLowerCase() === header,
      );
      return item && item.valid ? item.parsed_ip : remoteIp;
    }
    if (mode === 'xff') {
      const ips = ipDiagnosis.xff_ips || [];
      let index = parseInt(inputs['risk_control.xff_index'], 10);
      if (isNaN(index)) index = -1;
      if (index < 0) index = ips.length + index;
      if (index >= 0 && index < ips.length) return ips[index];
      return remoteIp;
    }
    const preview = (ipDiagnosis.mode_previews || []).find(
      (candidate) => candidate.mode === 'auto',
    );
    return preview ? preview.ip : '';
  }

  const ipModeLabels = {
    auto: t('自动（按常见代理头扫描）'),
    trusted_header: t('信任指定请求头'),
    xff: t('X-Forwarded-For 指定位置'),
    remote_addr: t('直连地址（RemoteAddr）'),
  };

  const ipModeDescriptions = {
    auto: t('按固定优先级扫描常见代理头，取第一个公网IP；多层CDN下可能取到边缘节点IP'),
    trusted_header: t('只信任指定的请求头，例如 Cloudflare 后填 CF-Connecting-IP'),
    xff: t('从 X-Forwarded-For 代理链中取指定位置的IP'),
    remote_addr: t('直接使用TCP连接来源地址，仅在客户端不经任何代理/CDN直连时正确'),
  };

  const currentIpMode = inputs['risk_control.ip_mode'];
  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('日志设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'LogConsumeEnabled'}
                  label={t('启用额度消费日志记录')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      LogConsumeEnabled: value,
                    });
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'ForceRecordIPEnabled'}
                  label={t('强制记录所有用户日志IP')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      ForceRecordIPEnabled: value,
                    });
                  }}
                  extraText={t(
                    '开启后，无论用户个人设置如何，所有消费和错误日志都将记录客户端IP地址',
                  )}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Spin spinning={loadingCleanHistoryLog}>
                  <Form.DatePicker
                    label={t('清除历史日志')}
                    field={'historyTimestamp'}
                    type='dateTime'
                    inputReadOnly={true}
                    onChange={(value) => {
                      setInputs({
                        ...inputs,
                        historyTimestamp: value,
                      });
                    }}
                  />
                  <Text
                    type='tertiary'
                    size='small'
                    style={{ display: 'block', marginTop: 4, marginBottom: 8 }}
                  >
                    {t('将清除选定时间之前的所有日志')}
                  </Text>
                  <Button
                    size='default'
                    type='danger'
                    onClick={onCleanHistoryLog}
                  >
                    {t('清除历史日志')}
                  </Button>
                </Spin>
              </Col>
            </Row>

            <Row gutter={16} style={{ marginTop: 8 }}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Select
                  field={'risk_control.ip_mode'}
                  label={t('客户端 IP 判定方式')}
                  style={{ width: '100%' }}
                  optionList={IP_MODES.map((mode) => ({
                    value: mode,
                    label: ipModeLabels[mode],
                  }))}
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      'risk_control.ip_mode': value,
                    });
                  }}
                  extraText={t(
                    '决定日志、限流、令牌IP白名单、地区限制与风控如何获取真实客户端IP',
                  )}
                />
                <Button
                  size='small'
                  theme='light'
                  type='primary'
                  style={{ marginTop: 4 }}
                  onClick={openIpDetectModal}
                >
                  {t('检测当前请求')}
                </Button>
              </Col>
              {currentIpMode === 'trusted_header' && (
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Select
                    field={'risk_control.trusted_ip_header'}
                    label={t('请求头名称')}
                    style={{ width: '100%' }}
                    filter
                    allowCreate
                    optionList={PRESET_IP_HEADERS.map((header) => ({
                      value: header,
                      label: header,
                    }))}
                    onChange={(value) => {
                      setInputs({
                        ...inputs,
                        'risk_control.trusted_ip_header': value,
                      });
                    }}
                    extraText={t(
                      '只信任指定的请求头，例如 Cloudflare 后填 CF-Connecting-IP',
                    )}
                  />
                </Col>
              )}
              {currentIpMode === 'xff' && (
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Input
                    field={'risk_control.xff_index'}
                    label={t('XFF 位置')}
                    type='number'
                    onChange={(value) => {
                      setInputs({
                        ...inputs,
                        'risk_control.xff_index': String(value),
                      });
                    }}
                    extraText={t(
                      '-1 表示最后一个，-2 表示倒数第二个，0 表示第一个（最左，可被客户端伪造）',
                    )}
                  />
                </Col>
              )}
            </Row>

            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存日志设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
      <Modal
        title={t('检测当前请求')}
        visible={showIpDetectModal}
        onCancel={() => setShowIpDetectModal(false)}
        footer={
          <Button onClick={() => setShowIpDetectModal(false)}>
            {t('关闭')}
          </Button>
        }
        centered
        width={720}
        style={{ maxWidth: '92vw' }}
        bodyStyle={{
          maxHeight: 'calc(80vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
      >
        <Spin spinning={ipDetectLoading}>
          <Banner
            type='info'
            description={t(
              '各方式解析出的当前请求 IP，选择与你真实 IP 一致的方式',
            )}
            style={{ marginBottom: 12 }}
          />
          <Table
            dataSource={IP_MODES.map((mode) => ({
              key: mode,
              mode,
              label: ipModeLabels[mode],
              description: ipModeDescriptions[mode],
              ip: previewIpForMode(mode),
            }))}
            pagination={false}
            columns={[
              {
                title: t('判定方式'),
                dataIndex: 'label',
                render: (label, record) => (
                  <div>
                    <div>
                      {label}{' '}
                      {record.mode === currentIpMode && (
                        <Tag color='green' size='small'>
                          {t('当前使用')}
                        </Tag>
                      )}
                    </div>
                    <Text type='tertiary' size='small'>
                      {record.description}
                    </Text>
                  </div>
                ),
              },
              {
                title: t('该方式解析出的当前 IP'),
                dataIndex: 'ip',
                width: 170,
                render: (ip) =>
                  ip ? <Text code>{ip}</Text> : <Text type='tertiary'>-</Text>,
              },
              {
                title: '',
                dataIndex: 'mode',
                width: 110,
                render: (mode) =>
                  mode === currentIpMode ? null : (
                    <Button
                      size='small'
                      theme='solid'
                      type='primary'
                      onClick={() => {
                        setInputs({
                          ...inputs,
                          'risk_control.ip_mode': mode,
                        });
                        refForm.current?.setValue('risk_control.ip_mode', mode);
                        setShowIpDetectModal(false);
                        showSuccess(
                          t('已选择判定方式，请记得点击「保存日志设置」'),
                        );
                      }}
                    >
                      {t('使用此方式')}
                    </Button>
                  ),
              },
            ]}
          />
          {ipDiagnosis && (ipDiagnosis.xff_ips || []).length > 0 && (
            <div style={{ marginTop: 12 }}>
              <Text type='tertiary' size='small'>
                {t('当前 X-Forwarded-For 链')}：
              </Text>
              <Text code size='small'>
                {ipDiagnosis.xff_ips.join(' → ')}
              </Text>
            </div>
          )}
        </Spin>
      </Modal>
    </>
  );
}
