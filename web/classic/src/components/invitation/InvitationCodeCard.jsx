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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Avatar,
  Badge,
  Button,
  Card,
  Empty,
  Space,
  Typography,
  Tag,
} from '@douyinfe/semi-ui';
import { KeyRound, Link as LinkIcon, RefreshCw, UserPlus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, copy, showError, showSuccess, timestamp2string } from '../../helpers';

const { Text } = Typography;

const isExpired = (record) =>
  record.expired_time !== 0 &&
  record.expired_time < Math.floor(Date.now() / 1000);

const isExhausted = (record) =>
  record.max_uses > 0 && record.used_count >= record.max_uses;

const renderStatus = (record, t) => {
  if (record.status !== 1) {
    return (
      <Tag color='red' shape='circle'>
        {t('已禁用')}
      </Tag>
    );
  }
  if (isExpired(record)) {
    return (
      <Tag color='orange' shape='circle'>
        {t('已过期')}
      </Tag>
    );
  }
  if (isExhausted(record)) {
    return (
      <Tag color='violet' shape='circle'>
        {t('已用尽')}
      </Tag>
    );
  }
  return (
    <Tag color='green' shape='circle'>
      {t('可邀请')}
    </Tag>
  );
};

const InvitationCodeCard = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [generating, setGenerating] = useState(false);
  const [quotaInfo, setQuotaInfo] = useState(null);
  const [invitationCodes, setInvitationCodes] = useState([]);

  const loadData = async () => {
    setLoading(true);
    try {
      const [quotaRes, codeRes] = await Promise.all([
        API.get('/api/user/invitation_codes/quota'),
        API.get('/api/user/invitation_codes'),
      ]);

      if (quotaRes.data.success) {
        setQuotaInfo(quotaRes.data.data);
      } else {
        showError(quotaRes.data.message);
      }

      if (codeRes.data.success) {
        setInvitationCodes(codeRes.data.data || []);
      } else {
        showError(codeRes.data.message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData().catch((error) => showError(error));
  }, []);

  const remainingText = useMemo(() => {
    if (!quotaInfo) {
      return '-';
    }
    if (quotaInfo.limit < 0) {
      return t('无限');
    }
    return String(quotaInfo.remaining);
  }, [quotaInfo, t]);

  const handleGenerate = async () => {
    setGenerating(true);
    try {
      const res = await API.post('/api/user/invitation_codes');
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(t('邀请码创建成功！'));
        if (data?.code) {
          await copy(`${window.location.origin}/register?code=${data.code}`);
        }
        await loadData();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setGenerating(false);
    }
  };

  const copyInvitationCode = async (code) => {
    if (await copy(code)) {
      showSuccess(t('邀请码已复制到剪贴板'));
      return;
    }
    showError(t('复制失败，请手动复制'));
  };

  const copyInvitationLink = async (code) => {
    const link = `${window.location.origin}/register?code=${code}`;
    if (await copy(link)) {
      showSuccess(t('邀请链接已复制到剪贴板'));
      return;
    }
    showError(t('复制失败，请手动复制'));
  };

  return (
    <Card className='!rounded-2xl shadow-sm border-0' loading={loading}>
      <div className='flex items-center justify-between mb-4 gap-3'>
        <div className='flex items-center'>
          <Avatar size='small' color='indigo' className='mr-3 shadow-md'>
            <UserPlus size={16} />
          </Avatar>
          <div>
            <Text className='text-lg font-medium'>{t('邀请码')}</Text>
            <div className='text-xs'>
              {t('生成专属注册链接，按策略控制可邀请人数')}
            </div>
          </div>
        </div>
        <Space>
          <Button
            type='tertiary'
            theme='outline'
            icon={<RefreshCw size={14} />}
            onClick={() => loadData()}
          >
            {t('刷新')}
          </Button>
          <Button
            type='primary'
            icon={<KeyRound size={14} />}
            loading={generating}
            disabled={!quotaInfo?.can_generate}
            onClick={handleGenerate}
          >
            {t('生成邀请码')}
          </Button>
        </Space>
      </div>

      <div className='grid grid-cols-1 md:grid-cols-3 gap-3 mb-4'>
        <Card className='!rounded-xl border-0 bg-slate-50'>
          <div className='text-xs text-gray-500'>{t('剩余生成次数')}</div>
          <div className='text-2xl font-semibold mt-1'>{remainingText}</div>
        </Card>
        <Card className='!rounded-xl border-0 bg-slate-50'>
          <div className='text-xs text-gray-500'>{t('默认最大使用次数')}</div>
          <div className='text-2xl font-semibold mt-1'>
            {quotaInfo?.default_code_max_uses === 0
              ? t('无限')
              : quotaInfo?.default_code_max_uses ?? '-'}
          </div>
        </Card>
        <Card className='!rounded-xl border-0 bg-slate-50'>
          <div className='text-xs text-gray-500'>{t('默认有效期')}</div>
          <div className='text-2xl font-semibold mt-1'>
            {quotaInfo?.default_code_valid_days === 0
              ? t('永久')
              : t('{{days}} 天', {
                  days: quotaInfo?.default_code_valid_days ?? 0,
                })}
          </div>
        </Card>
      </div>

      {quotaInfo?.reason && (
        <div className='mb-4'>
          <Badge dot type='warning' />
          <Text type='secondary' className='ml-2'>
            {quotaInfo.reason}
          </Text>
        </div>
      )}

      <div className='space-y-3'>
        {invitationCodes.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('暂无邀请码')}
          />
        ) : (
          invitationCodes.map((record) => (
            <Card
              key={record.id}
              className='!rounded-xl border border-slate-100 shadow-none'
              bodyStyle={{ padding: '16px' }}
            >
              <div className='flex flex-col gap-3'>
                <div className='flex items-center justify-between gap-2 flex-wrap'>
                  <div>
                    <Text strong>{record.code}</Text>
                    <div className='text-xs text-gray-500 mt-1'>
                      {record.name || t('用户邀请码')}
                    </div>
                  </div>
                  {renderStatus(record, t)}
                </div>

                <div className='grid grid-cols-1 md:grid-cols-3 gap-2 text-sm text-gray-600'>
                  <div>
                    {t('使用情况')}：
                    {record.used_count}/
                    {record.max_uses === 0 ? t('无限') : record.max_uses}
                  </div>
                  <div>
                    {t('创建时间')}：{timestamp2string(record.created_time)}
                  </div>
                  <div>
                    {t('过期时间')}：
                    {record.expired_time === 0
                      ? t('永不过期')
                      : timestamp2string(record.expired_time)}
                  </div>
                </div>

                <div className='flex flex-wrap gap-2'>
                  <Button size='small' onClick={() => copyInvitationCode(record.code)}>
                    {t('复制邀请码')}
                  </Button>
                  <Button
                    size='small'
                    type='tertiary'
                    icon={<LinkIcon size={14} />}
                    onClick={() => copyInvitationLink(record.code)}
                  >
                    {t('复制邀请链接')}
                  </Button>
                </div>
              </div>
            </Card>
          ))
        )}
      </div>
    </Card>
  );
};

export default InvitationCodeCard;
