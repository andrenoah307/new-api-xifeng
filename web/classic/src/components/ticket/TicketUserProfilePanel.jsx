import React, { useCallback, useMemo, useRef, useState } from 'react';
import {
  Button,
  Descriptions,
  Empty,
  Modal,
  Space,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconRefresh, IconUser } from '@douyinfe/semi-icons';
import CardTable from '../common/ui/CardTable';
import { API, showError, timestamp2string } from '../../helpers';
import { renderQuota } from '../../helpers/render';

const { Title, Text } = Typography;

const RoleLabel = (role, t) => {
  switch (Number(role)) {
    case 100:
      return t('超级管理员');
    case 10:
      return t('管理员');
    default:
      return t('普通用户');
  }
};

const StatusTag = (status, t) => {
  if (Number(status) === 1) {
    return <Tag color='green' shape='circle'>{t('启用')}</Tag>;
  }
  return <Tag color='red' shape='circle'>{t('禁用')}</Tag>;
};

// Modal 懒加载：首次打开才请求一次用户画像，避免对 LOG_DB 产生无谓压力；用 ref 记录是否请求过，
// 这样请求失败也不会被 React 状态循环重新触发，刷新由"刷新"按钮显式发起。
const TicketUserProfilePanel = ({ ticketId, username, userId, t }) => {
  const [visible, setVisible] = useState(false);
  const [loading, setLoading] = useState(false);
  const [profile, setProfile] = useState(null);
  const attemptedRef = useRef(false);

  const loadProfile = useCallback(async () => {
    if (!ticketId) return;
    setLoading(true);
    attemptedRef.current = true;
    try {
      const res = await API.get(`/api/ticket/admin/${ticketId}/user-profile`);
      if (res.data?.success) {
        setProfile(res.data.data || null);
      } else {
        showError(res.data?.message || t('加载用户信息失败'));
      }
    } catch (e) {
      showError(
        e?.response?.status === 404
          ? t('该功能需要后端更新后才能使用')
          : t('请求失败'),
      );
    } finally {
      setLoading(false);
    }
  }, [ticketId, t]);

  const open = useCallback(() => {
    setVisible(true);
    if (!attemptedRef.current && !loading) {
      loadProfile();
    }
  }, [loadProfile, loading]);
  const close = useCallback(() => setVisible(false), []);

  const basicRows = useMemo(() => {
    if (!profile) return [];
    const rows = [
      {
        key: t('用户'),
        value: `${profile.username || '-'} (UID: ${profile.user_id || '-'})`,
      },
      { key: t('显示名'), value: profile.display_name || '-' },
      { key: t('邮箱'), value: profile.email || '-' },
      { key: t('角色'), value: RoleLabel(profile.role, t) },
      { key: t('状态'), value: StatusTag(profile.status, t) },
      { key: t('分组'), value: profile.group || 'default' },
      {
        key: t('注册时间'),
        value: profile.created_time
          ? timestamp2string(profile.created_time)
          : '-',
      },
      {
        key: t('当前余额'),
        value: <Text strong>{renderQuota(profile.quota || 0)}</Text>,
      },
      { key: t('累计消耗'), value: renderQuota(profile.used_quota || 0) },
      { key: t('请求次数'), value: Number(profile.request_count || 0) },
    ];
    if (profile.pending_refund_quota > 0) {
      rows.push({
        key: t('待审核退款'),
        value: (
          <Space>
            <Text>{renderQuota(profile.pending_refund_quota)}</Text>
            <Tag color='orange' shape='circle'>
              {t('已从余额扣除，等待审核')}
            </Tag>
          </Space>
        ),
      });
    }
    return rows;
  }, [profile, t]);

  const logColumns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'created_at',
        key: 'created_at',
        width: 170,
        render: (v) => (v ? timestamp2string(v) : '-'),
      },
      {
        title: t('模型'),
        dataIndex: 'model_name',
        key: 'model_name',
        render: (v) => v || '-',
      },
      {
        title: t('令牌'),
        dataIndex: 'token_name',
        key: 'token_name',
        render: (v) => v || '-',
      },
      {
        title: t('消耗'),
        dataIndex: 'quota',
        key: 'quota',
        width: 110,
        render: (v) => renderQuota(Number(v || 0)),
      },
      {
        title: t('输入'),
        dataIndex: 'prompt_tokens',
        key: 'prompt_tokens',
        width: 90,
      },
      {
        title: t('输出'),
        dataIndex: 'completion_tokens',
        key: 'completion_tokens',
        width: 90,
      },
    ],
    [t],
  );

  const modelColumns = useMemo(
    () => [
      {
        title: t('模型'),
        dataIndex: 'model_name',
        key: 'model_name',
        render: (v) => v || '-',
      },
      {
        title: t('调用次数'),
        dataIndex: 'count',
        key: 'count',
        width: 120,
        render: (v) => Number(v || 0),
      },
      {
        title: t('消耗额度'),
        dataIndex: 'quota',
        key: 'quota',
        width: 140,
        render: (v) => renderQuota(Number(v || 0)),
      },
      {
        title: t('Token 数'),
        dataIndex: 'token_used',
        key: 'token_used',
        width: 120,
        render: (v) => Number(v || 0),
      },
    ],
    [t],
  );

  // pr-8 给 Semi Modal 自带的右上角关闭按钮留位，避免"刷新"按钮被它覆盖。
  const titleNode = (
    <div className='flex items-center justify-between w-full pr-8'>
      <Space>
        <Title heading={5} className='!mb-0'>
          {t('用户详情')}
        </Title>
        <Text type='secondary'>
          {username ? `${username}` : ''}
          {userId ? ` (UID: ${userId})` : ''}
        </Text>
      </Space>
      <Button
        theme='borderless'
        icon={<IconRefresh />}
        loading={loading}
        onClick={loadProfile}
      >
        {t('刷新')}
      </Button>
    </div>
  );

  return (
    <>
      <Button
        theme='borderless'
        icon={<IconUser />}
        onClick={open}
      >
        {t('查看用户画像')}
      </Button>

      <Modal
        title={titleNode}
        visible={visible}
        onCancel={close}
        footer={null}
        centered
        width={860}
        style={{ maxWidth: '92vw' }}
        bodyStyle={{
          maxHeight: 'calc(80vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
      >
        {loading && !profile ? (
          <div className='flex items-center justify-center py-10'>
            <Spin />
          </div>
        ) : !profile ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('暂无用户信息')}
            style={{ padding: 30 }}
          />
        ) : (
          <div className='flex flex-col gap-4'>
            <Descriptions data={basicRows} />

            <div>
              <Title heading={6} className='!mb-2'>
                {t('最近 API 调用')}
              </Title>
              <CardTable
                rowKey='id'
                columns={logColumns}
                dataSource={profile.recent_logs || []}
                hidePagination
                scroll={undefined}
                empty={
                  <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description={t('暂无 API 调用记录')}
                  />
                }
              />
            </div>

            <div>
              <Title heading={6} className='!mb-2'>
                {t('模型使用 TopN（近 30 天）')}
              </Title>
              <CardTable
                rowKey='model_name'
                columns={modelColumns}
                dataSource={profile.model_usage || []}
                hidePagination
                scroll={undefined}
                empty={
                  <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description={t('暂无模型使用记录')}
                  />
                }
              />
            </div>
          </div>
        )}
      </Modal>
    </>
  );
};

export default TicketUserProfilePanel;
