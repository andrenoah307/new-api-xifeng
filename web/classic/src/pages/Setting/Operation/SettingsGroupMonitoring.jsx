import React, { useEffect, useState, useRef } from 'react';
import {
  Button,
  Col,
  Form,
  Row,
  Spin,
  Select,
  TagInput,
  Typography,
} from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

export default function SettingsGroupMonitoring(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [refreshLoading, setRefreshLoading] = useState(false);
  const [availableGroups, setAvailableGroups] = useState([]);

  const [inputs, setInputs] = useState({
    'group_monitoring_setting.monitoring_groups': '',
    'group_monitoring_setting.group_display_order': '',
    'group_monitoring_setting.availability_period_minutes': 60,
    'group_monitoring_setting.cache_hit_period_minutes': 60,
    'group_monitoring_setting.aggregation_interval_minutes': 5,
    'group_monitoring_setting.availability_exclude_models': '',
    'group_monitoring_setting.cache_hit_exclude_models': '',
    'group_monitoring_setting.availability_exclude_keywords': '',
    'group_monitoring_setting.availability_exclude_status_codes': '',
    'group_monitoring_setting.cache_tokens_separate_groups': '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  function parseArrayField(value) {
    if (!value || value === '[]' || value === 'null') return [];
    let arr = [];
    if (typeof value === 'string') {
      try {
        const parsed = JSON.parse(value);
        if (Array.isArray(parsed)) arr = parsed;
      } catch {
        arr = value.split(',');
      }
    } else if (Array.isArray(value)) {
      arr = value;
    }
    return arr
      .map((v) => String(v).trim())
      .filter((v) => v && v !== 'null' && v !== 'undefined');
  }

  // Convert array to stored string
  function arrayToString(arr) {
    if (!arr || arr.length === 0) return '';
    return JSON.stringify(arr);
  }

  // Fetch available groups from backend
  useEffect(() => {
    const fetchGroups = async () => {
      try {
        const res = await API.get('/api/group/');
        if (res.data.success) {
          const data = res.data.data;
          const groupNames = (
            Array.isArray(data) ? data : Object.keys(data || {})
          ).filter((g) => g !== 'auto');
          setAvailableGroups(groupNames);
        }
      } catch {
        // Silently fail
      }
    };
    fetchGroups();
  }, []);

  // Sync from props
  useEffect(() => {
    const currentInputs = {};
    for (let key in inputs) {
      if (props.options && props.options[key] !== undefined) {
        currentInputs[key] = props.options[key];
      } else {
        currentInputs[key] = inputs[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    if (refForm.current) {
      refForm.current.setValues(currentInputs);
    }
  }, [props.options]);

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
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
        value: String(value),
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

  async function handleRefreshNow() {
    setRefreshLoading(true);
    try {
      const res = await API.post('/api/monitoring/admin/refresh');
      if (res.data.success) {
        showSuccess(t('刷新成功'));
      } else {
        showError(res.data.message || t('刷新失败'));
      }
    } catch {
      showError(t('刷新失败'));
    } finally {
      setRefreshLoading(false);
    }
  }

  const rawGroups = parseArrayField(
    inputs['group_monitoring_setting.monitoring_groups']
  );

  const resolvedGroups =
    availableGroups.length > 0
      ? rawGroups
          .map((g) => {
            if (availableGroups.includes(g)) return g;
            const idx = parseInt(g, 10);
            if (!isNaN(idx) && idx >= 0 && idx < availableGroups.length) {
              return availableGroups[idx];
            }
            return null;
          })
          .filter((g) => g !== null)
      : rawGroups;

  useEffect(() => {
    if (availableGroups.length === 0 || rawGroups.length === 0) return;
    const hasIndices = rawGroups.some(
      (g) => !availableGroups.includes(g) && /^\d+$/.test(g)
    );
    if (!hasIndices) return;
    const encoded = arrayToString(resolvedGroups);
    setInputs((prev) => ({
      ...prev,
      'group_monitoring_setting.monitoring_groups': encoded,
      'group_monitoring_setting.group_display_order': encoded,
    }));
  }, [availableGroups, inputs['group_monitoring_setting.monitoring_groups']]);

  const selectedGroups = resolvedGroups;

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('分组监控设置')}>
          {/* Monitoring groups selector */}
          <Row gutter={16}>
            <Col xs={24} sm={16}>
              <div style={{ marginBottom: 16 }}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('监控分组')}
                </Text>
                <Select
                  multiple
                  filter
                  style={{ width: '100%' }}
                  placeholder={t('选择需要监控的分组')}
                  value={selectedGroups}
                  onChange={(val) => {
                    const encoded = arrayToString(val);
                    setInputs({
                      ...inputs,
                      'group_monitoring_setting.monitoring_groups': encoded,
                      'group_monitoring_setting.group_display_order': encoded,
                    });
                  }}
                >
                  {availableGroups.map((g) => (
                    <Select.Option key={g} value={g}>
                      {g}
                    </Select.Option>
                  ))}
                </Select>
                <Text
                  type='tertiary'
                  size='small'
                  style={{ marginTop: 4, display: 'block' }}
                >
                  {t('拖拽标签可调整显示顺序')}
                </Text>
              </div>
              {/* Draggable tag ordering */}
              {selectedGroups.length > 0 && (
                <TagInput
                  value={selectedGroups}
                  onChange={(val) => {
                    const encoded = arrayToString(val);
                    setInputs({
                      ...inputs,
                      'group_monitoring_setting.monitoring_groups': encoded,
                      'group_monitoring_setting.group_display_order': encoded,
                    });
                  }}
                  draggable
                  placeholder={t('拖拽调整顺序')}
                  style={{ width: '100%', marginBottom: 16 }}
                />
              )}
            </Col>
          </Row>

          {/* Numeric inputs */}
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                label={t('可用率统计周期')}
                step={1}
                min={1}
                suffix={t('分钟')}
                extraText={t('统计可用率的时间窗口')}
                field={'group_monitoring_setting.availability_period_minutes'}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'group_monitoring_setting.availability_period_minutes':
                      parseInt(value) || 60,
                  })
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                label={t('缓存命中率统计周期')}
                step={1}
                min={1}
                suffix={t('分钟')}
                extraText={t('统计缓存命中率的时间窗口')}
                field={'group_monitoring_setting.cache_hit_period_minutes'}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'group_monitoring_setting.cache_hit_period_minutes':
                      parseInt(value) || 60,
                  })
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                label={t('聚合间隔')}
                step={1}
                min={1}
                suffix={t('分钟')}
                extraText={t('历史数据聚合的时间间隔')}
                field={'group_monitoring_setting.aggregation_interval_minutes'}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'group_monitoring_setting.aggregation_interval_minutes':
                      parseInt(value) || 5,
                  })
                }
              />
            </Col>
          </Row>

          {/* Exclude models / keywords */}
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <div style={{ marginBottom: 16 }}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('可用率排除模型')}
                </Text>
                <TagInput
                  value={parseArrayField(
                    inputs['group_monitoring_setting.availability_exclude_models']
                  )}
                  onChange={(val) =>
                    setInputs({
                      ...inputs,
                      'group_monitoring_setting.availability_exclude_models':
                        arrayToString(val),
                    })
                  }
                  placeholder={t('输入后回车添加')}
                  style={{ width: '100%' }}
                />
                <Text
                  type='tertiary'
                  size='small'
                  style={{ marginTop: 4, display: 'block' }}
                >
                  {t('这些模型不计入可用率统计')}
                </Text>
              </div>
            </Col>
            <Col xs={24} sm={12}>
              <div style={{ marginBottom: 16 }}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('缓存命中率排除模型')}
                </Text>
                <TagInput
                  value={parseArrayField(
                    inputs['group_monitoring_setting.cache_hit_exclude_models']
                  )}
                  onChange={(val) =>
                    setInputs({
                      ...inputs,
                      'group_monitoring_setting.cache_hit_exclude_models':
                        arrayToString(val),
                    })
                  }
                  placeholder={t('输入后回车添加')}
                  style={{ width: '100%' }}
                />
                <Text
                  type='tertiary'
                  size='small'
                  style={{ marginTop: 4, display: 'block' }}
                >
                  {t('这些模型不计入缓存命中率统计')}
                </Text>
              </div>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <div style={{ marginBottom: 16 }}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('可用率排除关键词')}
                </Text>
                <TagInput
                  value={parseArrayField(
                    inputs['group_monitoring_setting.availability_exclude_keywords']
                  )}
                  onChange={(val) =>
                    setInputs({
                      ...inputs,
                      'group_monitoring_setting.availability_exclude_keywords':
                        arrayToString(val),
                    })
                  }
                  placeholder={t('输入后回车添加')}
                  style={{ width: '100%' }}
                />
                <Text
                  type='tertiary'
                  size='small'
                  style={{ marginTop: 4, display: 'block' }}
                >
                  {t('包含这些关键词的错误不计入不可用')}
                </Text>
              </div>
            </Col>
            <Col xs={24} sm={12}>
              <div style={{ marginBottom: 16 }}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('可用率排除状态码')}
                </Text>
                <TagInput
                  value={parseArrayField(
                    inputs['group_monitoring_setting.availability_exclude_status_codes']
                  )}
                  onChange={(val) =>
                    setInputs({
                      ...inputs,
                      'group_monitoring_setting.availability_exclude_status_codes':
                        arrayToString(val),
                    })
                  }
                  placeholder={t('输入状态码后回车，如 400、503')}
                  style={{ width: '100%' }}
                />
                <Text
                  type='tertiary'
                  size='small'
                  style={{ marginTop: 4, display: 'block' }}
                >
                  {t('这些HTTP状态码的错误不计入不可用')}
                </Text>
              </div>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <div style={{ marginBottom: 16 }}>
                <Text strong style={{ display: 'block', marginBottom: 8 }}>
                  {t('缓存Token独立分组')}
                </Text>
                <TagInput
                  value={parseArrayField(
                    inputs['group_monitoring_setting.cache_tokens_separate_groups']
                  )}
                  onChange={(val) =>
                    setInputs({
                      ...inputs,
                      'group_monitoring_setting.cache_tokens_separate_groups':
                        arrayToString(val),
                    })
                  }
                  placeholder={t('输入后回车添加')}
                  style={{ width: '100%' }}
                />
                <Text
                  type='tertiary'
                  size='small'
                  style={{ marginTop: 4, display: 'block' }}
                >
                  {t('这些分组的缓存Token单独统计')}
                </Text>
              </div>
            </Col>
          </Row>

          {/* Buttons */}
          <Row>
            <div style={{ display: 'flex', gap: 12 }}>
              <Button size='default' onClick={onSubmit}>
                {t('保存分组监控设置')}
              </Button>
              <Button
                size='default'
                type='tertiary'
                loading={refreshLoading}
                onClick={handleRefreshNow}
              >
                {t('立即刷新监控数据')}
              </Button>
            </div>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
