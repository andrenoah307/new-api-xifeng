import React from 'react';
import { Tag } from '@douyinfe/semi-ui';
import {
  getTicketStatusColor,
  getTicketStatusText,
} from './ticketUtils';

const TicketStatusTag = ({ status, t, size = 'default' }) => {
  return (
    <Tag color={getTicketStatusColor(status)} shape='circle' size={size}>
      {getTicketStatusText(status, t)}
    </Tag>
  );
};

export default TicketStatusTag;

