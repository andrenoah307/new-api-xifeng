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
  Avatar,
  Button,
  Card,
  Col,
  Form,
  Row,
  SideSheet,
  Space,
  Spin,
  Tag,
  Typography,
  Modal,
} from '@douyinfe/semi-ui';
import {
  IconClose,
  IconGift,
  IconSave,
  IconUser,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, downloadTextAsFile, showError, showSuccess } from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import { INVITATION_CODE_STATUS } from '../index';

const { Text, Title } = Typography;

const EditInvitationCodeModal = ({
  refresh,
  editingInvitationCode,
  visible,
  handleClose,
}) => {
  const { t } = useTranslation();
  const isEdit = editingInvitationCode.id !== undefined;
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [loading, setLoading] = useState(isEdit);

  const getInitValues = () => ({
    name: '',
    count: 1,
    max_uses: 1,
    owner_user_id: 0,
    expired_time: null,
  });

  const loadInvitationCode = async () => {
    setLoading(true);
    try {
      const res = await API.get(`/api/invitation_code/${editingInvitationCode.id}`);
      const { success, message, data } = res.data;
      if (success) {
        const nextValues = {
          ...getInitValues(),
          ...data,
          expired_time:
            data.expired_time && data.expired_time !== 0
              ? new Date(data.expired_time * 1000)
              : null,
        };
        formApiRef.current?.setValues(nextValues);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!formApiRef.current) {
      return;
    }
    if (isEdit) {
      loadInvitationCode().catch((error) => showError(error));
      return;
    }
    setLoading(false);
    formApiRef.current.setValues(getInitValues());
  }, [editingInvitationCode.id]);

  const submit = async (values) => {
    const payload = {
      ...values,
      count: parseInt(values.count, 10) || 1,
      max_uses: parseInt(values.max_uses, 10) || 0,
      owner_user_id: parseInt(values.owner_user_id, 10) || 0,
      expired_time: values.expired_time
        ? Math.floor(values.expired_time.getTime() / 1000)
        : 0,
    };

    setLoading(true);
    try {
      let res;
      if (isEdit) {
        res = await API.put('/api/invitation_code/', {
          ...payload,
          id: parseInt(editingInvitationCode.id, 10),
          status:
            editingInvitationCode.status || INVITATION_CODE_STATUS.ENABLED,
        });
      } else {
        res = await API.post('/api/invitation_code/', payload);
      }
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }

      if (isEdit) {
        showSuccess(t('邀请码更新成功！'));
      } else {
        showSuccess(t('邀请码创建成功！'));
      }

      await refresh(isEdit ? undefined : 1);
      handleClose();

      if (!isEdit && Array.isArray(data) && data.length > 0) {
        const text = data.join('\n');
        Modal.confirm({
          title: t('邀请码创建成功'),
          content: t('邀请码创建成功，是否下载邀请码列表？'),
          onOk: () => downloadTextAsFile(text, `${payload.name || 'invitation-codes'}.txt`),
        });
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <SideSheet
      placement={isEdit ? 'right' : 'left'}
      visible={visible}
      width={isMobile ? '100%' : 600}
      bodyStyle={{ padding: 0 }}
      closeIcon={null}
      onCancel={handleClose}
      title={
        <Space>
          <Tag color={isEdit ? 'blue' : 'green'} shape='circle'>
            {isEdit ? t('更新') : t('新建')}
          </Tag>
          <Title heading={4} className='m-0'>
            {isEdit ? t('更新邀请码信息') : t('创建新的邀请码')}
          </Title>
        </Space>
      }
      footer={
        <div className='flex justify-end bg-white'>
          <Space>
            <Button
              theme='solid'
              icon={<IconSave />}
              loading={loading}
              onClick={() => formApiRef.current?.submitForm()}
            >
              {t('提交')}
            </Button>
            <Button
              theme='light'
              type='primary'
              icon={<IconClose />}
              onClick={handleClose}
            >
              {t('取消')}
            </Button>
          </Space>
        </div>
      }
    >
      <Spin spinning={loading}>
        <Form
          initValues={getInitValues()}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={submit}
        >
          <div className='p-2'>
            <Card className='!rounded-2xl shadow-sm border-0 mb-6'>
              <div className='flex items-center mb-2'>
                <Avatar size='small' color='blue' className='mr-2 shadow-md'>
                  <IconGift size={16} />
                </Avatar>
                <div>
                  <Text className='text-lg font-medium'>{t('基本信息')}</Text>
                  <div className='text-xs text-gray-600'>
                    {t('设置邀请码名称、归属人和有效期')}
                  </div>
                </div>
              </div>

              <Row gutter={12}>
                <Col span={24}>
                  <Form.Input
                    field='name'
                    label={t('名称')}
                    placeholder={t('请输入名称')}
                    rules={[{ required: true, message: t('请输入名称') }]}
                    showClear
                  />
                </Col>
                <Col span={24}>
                  <Form.DatePicker
                    field='expired_time'
                    label={t('过期时间')}
                    type='dateTime'
                    placeholder={t('选择过期时间（可选，留空为永久）')}
                    showClear
                    style={{ width: '100%' }}
                  />
                </Col>
              </Row>
            </Card>

            <Card className='!rounded-2xl shadow-sm border-0'>
              <div className='flex items-center mb-2'>
                <Avatar size='small' color='green' className='mr-2 shadow-md'>
                  <IconUser size={16} />
                </Avatar>
                <div>
                  <Text className='text-lg font-medium'>{t('使用设置')}</Text>
                  <div className='text-xs text-gray-600'>
                    {t('配置使用次数、生成数量和归属用户')}
                  </div>
                </div>
              </div>

              <Row gutter={12}>
                <Col span={12}>
                  <Form.InputNumber
                    field='max_uses'
                    label={t('最大使用次数')}
                    min={0}
                    placeholder={t('0 表示无限')}
                    style={{ width: '100%' }}
                  />
                </Col>
                <Col span={12}>
                  <Form.InputNumber
                    field='owner_user_id'
                    label={t('所有者用户ID')}
                    min={0}
                    placeholder={t('留空或 0 表示无归属')}
                    style={{ width: '100%' }}
                  />
                </Col>
                {!isEdit && (
                  <Col span={12}>
                    <Form.InputNumber
                      field='count'
                      label={t('生成数量')}
                      min={1}
                      placeholder={t('请输入生成数量')}
                      rules={[{ required: true, message: t('请输入生成数量') }]}
                      style={{ width: '100%' }}
                    />
                  </Col>
                )}
              </Row>
            </Card>
          </div>
        </Form>
      </Spin>
    </SideSheet>
  );
};

export default EditInvitationCodeModal;
