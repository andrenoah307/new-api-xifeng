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
import { Button, Col, Form, Row, Space, Spin, Switch } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { parseModelNameRPMConfig } from '../../../helpers/model-name-rpm';
import { useTranslation } from 'react-i18next';
import ModelNameRPMVisualEditor from './ModelNameRPMVisualEditor';

const MODEL_NAME_RPM_OPTION_KEY = 'ModelNameRPMRateLimit';
const DEFAULT_MODEL_NAME_RPM_RATE_LIMIT = JSON.stringify(
  {
    enabled: false,
    models: {},
  },
  null,
  2,
);

function getModelNameRPMEnabled(value) {
  try {
    const config = JSON.parse(value);
    return (
      config !== null &&
      typeof config === 'object' &&
      !Array.isArray(config) &&
      config.enabled === true
    );
  } catch {
    return false;
  }
}

export default function RequestRateLimit(props) {
  const { t } = useTranslation();

  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ModelRequestRateLimitEnabled: false,
    ModelRequestRateLimitCount: -1,
    ModelRequestRateLimitSuccessCount: 1000,
    ModelRequestRateLimitDurationMinutes: 1,
    ModelRequestRateLimitGroup: '',
    [MODEL_NAME_RPM_OPTION_KEY]: DEFAULT_MODEL_NAME_RPM_RATE_LIMIT,
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  // null means "not chosen yet": the mode then follows whether the stored
  // document can be represented visually without rewriting it.
  const [modelNameRPMEditMode, setModelNameRPMEditMode] = useState(null);
  const prevModelNameRPMEditModeRef = useRef(null);

  const modelNameRPMValue = inputs[MODEL_NAME_RPM_OPTION_KEY];
  const modelNameRPMEditModeResolved =
    modelNameRPMEditMode ??
    (parseModelNameRPMConfig(modelNameRPMValue).ok ? 'visual' : 'json');

  function setModelNameRPMValue(nextValue) {
    setInputs((prev) => ({
      ...prev,
      [MODEL_NAME_RPM_OPTION_KEY]: nextValue,
    }));
    if (refForm.current) {
      // Use setValue instead of setValues. Semi Form's setValues assigns
      // undefined to every registered field not included in the payload.
      refForm.current.setValue(MODEL_NAME_RPM_OPTION_KEY, nextValue);
    }
  }

  function switchToModelNameRPMVisualMode() {
    if (!parseModelNameRPMConfig(modelNameRPMValue).ok) {
      showError(t('Fix the JSON before switching to the visual editor.'));
      return;
    }
    setModelNameRPMEditMode('visual');
  }

  function onModelNameRPMEnabledChange(value) {
    const currentValue = inputs[MODEL_NAME_RPM_OPTION_KEY];
    if (!verifyJSON(currentValue)) {
      showError(t('不是合法的 JSON 字符串'));
      return;
    }

    try {
      const config = JSON.parse(currentValue);
      if (
        config === null ||
        typeof config !== 'object' ||
        Array.isArray(config)
      ) {
        return;
      }

      config.enabled = value;
      setModelNameRPMValue(JSON.stringify(config, null, 2));
    } catch {
      showError(t('不是合法的 JSON 字符串'));
    }
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    let modelNameRPMValue = inputs[MODEL_NAME_RPM_OPTION_KEY];
    if (updateArray.some((item) => item.key === MODEL_NAME_RPM_OPTION_KEY)) {
      if (!verifyJSON(modelNameRPMValue)) {
        return showError(t('不是合法的 JSON 字符串'));
      }

      // Keep the switch and JSON in sync while leaving semantic validation to the backend.
      try {
        const config = JSON.parse(modelNameRPMValue);
        if (
          config !== null &&
          typeof config === 'object' &&
          !Array.isArray(config)
        ) {
          config.enabled = getModelNameRPMEnabled(modelNameRPMValue);
          modelNameRPMValue = JSON.stringify(config, null, 2);
        }
      } catch {
        return showError(t('不是合法的 JSON 字符串'));
      }
    }

    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (item.key === MODEL_NAME_RPM_OPTION_KEY) {
        value = modelNameRPMValue;
      } else if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else {
        value = inputs[item.key];
      }
      return API.put('/api/option/', {
        key:
          item.key === MODEL_NAME_RPM_OPTION_KEY
            ? 'ModelNameRPMRateLimit'
            : item.key,
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

        for (let i = 0; i < res.length; i++) {
          if (!res[i].data.success) {
            return showError(res[i].data.message);
          }
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

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        if (
          key === MODEL_NAME_RPM_OPTION_KEY &&
          typeof props.options[key] === 'string'
        ) {
          try {
            currentInputs[key] = JSON.stringify(
              JSON.parse(props.options[key]),
              null,
              2,
            );
          } catch {
            currentInputs[key] = props.options[key];
          }
        } else {
          currentInputs[key] = props.options[key];
        }
      }
    }
    if (
      !Object.prototype.hasOwnProperty.call(
        currentInputs,
        MODEL_NAME_RPM_OPTION_KEY,
      )
    ) {
      currentInputs[MODEL_NAME_RPM_OPTION_KEY] =
        DEFAULT_MODEL_NAME_RPM_RATE_LIMIT;
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  useEffect(() => {
    // The JSON textarea is unmounted in visual mode, so Semi drops any value
    // written while it was hidden. Re-seed it right after it mounts again.
    const previousMode = prevModelNameRPMEditModeRef.current;
    prevModelNameRPMEditModeRef.current = modelNameRPMEditModeResolved;
    if (previousMode === modelNameRPMEditModeResolved) return;
    if (modelNameRPMEditModeResolved !== 'json') return;
    if (!refForm.current) return;
    refForm.current.setValue(MODEL_NAME_RPM_OPTION_KEY, modelNameRPMValue);
  }, [modelNameRPMEditModeResolved, modelNameRPMValue]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('模型请求速率限制')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'ModelRequestRateLimitEnabled'}
                  label={t('启用用户模型请求速率限制（可能会影响高并发性能）')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitEnabled: value,
                    });
                  }}
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('限制周期')}
                  step={1}
                  min={0}
                  suffix={t('分钟')}
                  extraText={t('频率限制的周期（分钟）')}
                  field={'ModelRequestRateLimitDurationMinutes'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitDurationMinutes: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('用户每周期最多请求次数')}
                  step={1}
                  min={0}
                  max={100000000}
                  suffix={t('次')}
                  extraText={t('包括失败请求的次数，0代表不限制')}
                  field={'ModelRequestRateLimitCount'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitCount: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('用户每周期最多请求完成次数')}
                  step={1}
                  min={1}
                  max={100000000}
                  suffix={t('次')}
                  extraText={t('只包括请求成功的次数')}
                  field={'ModelRequestRateLimitSuccessCount'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitSuccessCount: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('分组速率限制')}
                  placeholder={t(
                    '{\n  "default": [200, 100],\n  "vip": [0, 1000]\n}',
                  )}
                  field={'ModelRequestRateLimitGroup'}
                  autosize={{ minRows: 5, maxRows: 15 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  extraText={
                    <div>
                      <p>{t('说明：')}</p>
                      <ul>
                        <li>
                          {t(
                            '使用 JSON 对象格式，格式为：{"组名": [最多请求次数, 最多请求完成次数]}',
                          )}
                        </li>
                        <li>
                          {t(
                            '示例：{"default": [200, 100], "vip": [0, 1000]}。',
                          )}
                        </li>
                        <li>
                          {t(
                            '[最多请求次数]必须大于等于0，[最多请求完成次数]必须大于等于1。',
                          )}
                        </li>
                        <li>
                          {t(
                            '[最多请求次数]和[最多请求完成次数]的最大值为2147483647。',
                          )}
                        </li>
                        <li>{t('分组速率配置优先级高于全局速率限制。')}</li>
                        <li>{t('限制周期统一使用上方配置的“限制周期”值。')}</li>
                      </ul>
                    </div>
                  }
                  onChange={(value) => {
                    setInputs({ ...inputs, ModelRequestRateLimitGroup: value });
                  }}
                />
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存模型速率限制')}
              </Button>
            </Row>
          </Form.Section>

          <Form.Section text={t('Model name RPM rate limiting')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Slot label={t('Enable model name RPM rate limiting')}>
                  <Switch
                    checked={getModelNameRPMEnabled(
                      inputs[MODEL_NAME_RPM_OPTION_KEY],
                    )}
                    size='default'
                    checkedText='｜'
                    uncheckedText='〇'
                    aria-label={t('Enable model name RPM rate limiting')}
                    onChange={onModelNameRPMEnabledChange}
                  />
                </Form.Slot>
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={24} md={16} lg={16} xl={16}>
                <Space style={{ marginBottom: 10 }}>
                  <Button
                    type={
                      modelNameRPMEditModeResolved === 'visual'
                        ? 'primary'
                        : 'tertiary'
                    }
                    onClick={switchToModelNameRPMVisualMode}
                  >
                    {t('可视化')}
                  </Button>
                  <Button
                    type={
                      modelNameRPMEditModeResolved === 'json'
                        ? 'primary'
                        : 'tertiary'
                    }
                    onClick={() => setModelNameRPMEditMode('json')}
                  >
                    {t('JSON 模式')}
                  </Button>
                </Space>

                {modelNameRPMEditModeResolved === 'visual' ? (
                  <ModelNameRPMVisualEditor
                    value={modelNameRPMValue}
                    onChange={setModelNameRPMValue}
                  />
                ) : (
                  <Form.TextArea
                    label={t('Model name RPM configuration')}
                    placeholder={t('Model name RPM configuration example')}
                    field={MODEL_NAME_RPM_OPTION_KEY}
                    autosize={{ minRows: 10, maxRows: 20 }}
                    trigger='blur'
                    stopValidateWithError
                    style={{ fontFamily: 'monospace' }}
                    rules={[
                      {
                        validator: (rule, value) => verifyJSON(value),
                        message: t('不是合法的 JSON 字符串'),
                      },
                    ]}
                    onChange={(value) => {
                      setInputs((prev) => ({
                        ...prev,
                        [MODEL_NAME_RPM_OPTION_KEY]: value,
                      }));
                    }}
                  />
                )}

                <div style={{ marginTop: 8 }}>
                  <p>{t('说明：')}</p>
                  <ul>
                    <li>
                      {t(
                        'Models not listed here are not subject to this limit.',
                      )}
                    </li>
                    <li>
                      {t(
                        'Group limits are stricter sub-limits of the global limit; both apply to each request (one request uses both the global and group buckets).',
                      )}
                    </li>
                    <li>
                      {t(
                        'global_rpm must be a positive integer. Delete a model rule to disable it; set enabled to false to disable all rules.',
                      )}
                    </li>
                  </ul>
                </div>
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('Save model name RPM rate limit')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
