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
  Modal,
  Form,
  Spin,
  Row,
  Col,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const EditDiscountCode = ({ visible, record, onClose, onSuccess }) => {
  const { t } = useTranslation();
  const isEdit = record && record.id != null;
  const [loading, setLoading] = useState(false);
  const formApiRef = useRef(null);

  const getInitValues = () => ({
    name: '',
    code: '',
    discount_rate: 90,
    start_time: null,
    end_time: null,
    max_uses_per_user: 0,
    max_uses_total: 0,
    count: 1,
  });

  useEffect(() => {
    if (!visible) return;
    if (isEdit) {
      loadRecord();
    } else if (formApiRef.current) {
      formApiRef.current.setValues(getInitValues());
    }
  }, [visible, record]);

  const loadRecord = async () => {
    setLoading(true);
    try {
      const res = await API.get(`/api/discount_code/${record.id}`);
      const { success, message, data } = res.data;
      if (success) {
        const values = {
          name: data.name || '',
          code: data.code || '',
          discount_rate: data.discount_rate || 90,
          start_time: data.start_time && data.start_time > 0
            ? new Date(data.start_time * 1000)
            : null,
          end_time: data.end_time && data.end_time > 0
            ? new Date(data.end_time * 1000)
            : null,
          max_uses_per_user: data.max_uses_per_user || 0,
          max_uses_total: data.max_uses_total || 0,
          count: 1,
        };
        formApiRef.current?.setValues(values);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  const handleSubmit = async (values) => {
    setLoading(true);
    try {
      const payload = {
        name: values.name || '',
        code: values.code || '',
        discount_rate: parseInt(values.discount_rate) || 90,
        start_time: values.start_time
          ? Math.floor(values.start_time.getTime() / 1000)
          : 0,
        end_time: values.end_time
          ? Math.floor(values.end_time.getTime() / 1000)
          : 0,
        max_uses_per_user: parseInt(values.max_uses_per_user) || 0,
        max_uses_total: parseInt(values.max_uses_total) || 0,
      };

      let res;
      if (isEdit) {
        payload.id = record.id;
        res = await API.put('/api/discount_code/', payload);
      } else {
        payload.count = parseInt(values.count) || 1;
        res = await API.post('/api/discount_code/', payload);
      }

      const { success, message } = res.data;
      if (success) {
        showSuccess(
          isEdit
            ? t('折扣码更新成功！')
            : t('折扣码创建成功！'),
        );
        onSuccess();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  return (
    <Modal
      title={isEdit ? t('编辑折扣码') : t('创建折扣码')}
      visible={visible}
      onOk={() => formApiRef.current?.submitForm()}
      onCancel={onClose}
      okText={t('提交')}
      cancelText={t('取消')}
      confirmLoading={loading}
      centered
      width={600}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
    >
      <Spin spinning={loading}>
        <Form
          initValues={getInitValues()}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={handleSubmit}
          labelPosition='top'
        >
          <Row gutter={12}>
            <Col span={24}>
              <Form.Input
                field='name'
                label={t('名称')}
                placeholder={t('请输入名称')}
                maxLength={100}
                showClear
                style={{ width: '100%' }}
              />
            </Col>
            <Col span={24}>
              <Form.Input
                field='code'
                label={t('折扣码')}
                placeholder={t('折扣码内容留空则自动生成')}
                showClear
                style={{ width: '100%' }}
                extraText={t('折扣码内容留空则自动生成')}
              />
            </Col>
            <Col span={24}>
              <Form.InputNumber
                field='discount_rate'
                label={t('折扣率')}
                min={1}
                max={99}
                step={1}
                precision={0}
                style={{ width: '100%' }}
                rules={[
                  { required: true, message: t('请输入折扣率') },
                  {
                    validator: (_, v) => {
                      const num = parseInt(v);
                      return num >= 1 && num <= 99
                        ? Promise.resolve()
                        : Promise.reject(t('折扣率必须在1-99之间'));
                    },
                  },
                ]}
                extraText={t('90 = 九折，实付90%')}
              />
            </Col>
            <Col span={12}>
              <Form.DatePicker
                field='start_time'
                label={t('开始时间')}
                type='dateTime'
                placeholder={t('选择开始时间')}
                style={{ width: '100%' }}
                showClear
              />
            </Col>
            <Col span={12}>
              <Form.DatePicker
                field='end_time'
                label={t('结束时间')}
                type='dateTime'
                placeholder={t('选择结束时间')}
                style={{ width: '100%' }}
                showClear
              />
            </Col>
            <Col span={12}>
              <Form.InputNumber
                field='max_uses_per_user'
                label={t('单用户使用次数')}
                min={0}
                step={1}
                precision={0}
                style={{ width: '100%' }}
                extraText={t('0 表示无限')}
              />
            </Col>
            <Col span={12}>
              <Form.InputNumber
                field='max_uses_total'
                label={t('总使用次数')}
                min={0}
                step={1}
                precision={0}
                style={{ width: '100%' }}
                extraText={t('0 表示无限')}
              />
            </Col>
            {!isEdit && (
              <Col span={24}>
                <Form.InputNumber
                  field='count'
                  label={t('批量创建数量')}
                  min={1}
                  max={100}
                  step={1}
                  precision={0}
                  style={{ width: '100%' }}
                  rules={[
                    { required: true, message: t('请输入数量') },
                  ]}
                  extraText={t('批量创建数量') + '，1-100'}
                />
              </Col>
            )}
          </Row>
        </Form>
      </Spin>
    </Modal>
  );
};

export default EditDiscountCode;
