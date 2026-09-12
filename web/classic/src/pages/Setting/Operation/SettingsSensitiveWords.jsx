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
import { Button, Col, Form, Row, Spin, Tag } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsSensitiveWords(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    CheckSensitiveEnabled: false,
    CheckSensitiveOnPromptEnabled: false,
    SensitiveWords: '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  async function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    const fieldLabels = {
      CheckSensitiveEnabled: t('启用屏蔽词过滤功能'),
      CheckSensitiveOnPromptEnabled: t('启用 Prompt 检查'),
      SensitiveWords: t('屏蔽词列表'),
    };
    const savedLabels = [];

    setLoading(true);
    try {
      for (const item of updateArray) {
        const value =
          typeof inputs[item.key] === 'boolean'
            ? String(inputs[item.key])
            : inputs[item.key];
        const fieldLabel = fieldLabels[item.key] || item.key;
        let response;

        try {
          response = await API.put('/api/option/', {
            key: item.key,
            value,
          });
        } catch (error) {
          const message =
            error?.response?.data?.message ||
            error?.message ||
            t('保存失败，请重试');
          if (savedLabels.length > 0) {
            await props.refresh();
            showError(
              t(
                '以下设置已生效：{{items}}。保存“{{failed}}”失败：{{message}}',
                {
                  items: savedLabels.join('、'),
                  failed: fieldLabel,
                  message,
                },
              ),
            );
          } else {
            showError(message);
          }
          return;
        }

        if (!response?.data?.success) {
          const message = response?.data?.message || t('保存失败，请重试');
          if (savedLabels.length > 0) {
            await props.refresh();
            showError(
              t(
                '以下设置已生效：{{items}}。保存“{{failed}}”失败：{{message}}',
                {
                  items: savedLabels.join('、'),
                  failed: fieldLabel,
                  message,
                },
              ),
            );
          } else {
            showError(message);
          }
          return;
        }

        savedLabels.push(fieldLabel);
      }

      showSuccess(t('保存成功'));
      await props.refresh();
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);
  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('屏蔽词过滤设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'CheckSensitiveEnabled'}
                  label={t('启用屏蔽词过滤功能')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      CheckSensitiveEnabled: value,
                    });
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'CheckSensitiveOnPromptEnabled'}
                  label={t('启用 Prompt 检查')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      CheckSensitiveOnPromptEnabled: value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.TextArea
                  label={t('屏蔽词列表')}
                  extraText={t('一行一个屏蔽词，不需要符号分割')}
                  placeholder={t('一行一个屏蔽词，不需要符号分割')}
                  field={'SensitiveWords'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      SensitiveWords: value,
                    })
                  }
                  style={{ fontFamily: 'JetBrains Mono, Consolas' }}
                  autosize={{ minRows: 6, maxRows: 12 }}
                />
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存屏蔽词过滤设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
