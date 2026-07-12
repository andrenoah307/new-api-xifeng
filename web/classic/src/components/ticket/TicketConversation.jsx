import React, { useEffect, useMemo, useRef } from 'react';
import { Card, Empty, Spin, Typography } from '@douyinfe/semi-ui';
import TicketMessageItem from './TicketMessageItem';

const { Title } = Typography;

const TicketConversation = ({
  messages = [],
  currentUserId,
  loading = false,
  t,
}) => {
  const bottomRef = useRef(null);

  const normalizedMessages = useMemo(() => messages || [], [messages]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
  }, [normalizedMessages.length]);

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-center justify-between mb-4'>
        <Title heading={5} className='!mb-0'>
          {t('工单对话')}
        </Title>
      </div>
      <Spin spinning={loading}>
        {normalizedMessages.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('暂无工单消息')}
          />
        ) : (
          <div className='flex flex-col gap-3'>
            {normalizedMessages.map((message) => (
              <TicketMessageItem
                key={message.id}
                message={message}
                isMine={Number(message?.user_id) === Number(currentUserId)}
                t={t}
              />
            ))}
            <div ref={bottomRef} />
          </div>
        )}
      </Spin>
    </Card>
  );
};

export default TicketConversation;

