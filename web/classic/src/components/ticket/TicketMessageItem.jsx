import React from 'react';
import { Image, ImagePreview, Space, Tag, Typography } from '@douyinfe/semi-ui';
import { IconFile } from '@douyinfe/semi-icons';
import { timestamp2string } from '../../helpers';

const { Text, Paragraph } = Typography;

const humanSize = (bytes) => {
  const n = Number(bytes) || 0;
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
};

const isImageMime = (mime) => typeof mime === 'string' && mime.toLowerCase().startsWith('image/');

// 将附件 id 映射到后端下载地址。
//   - inline=1 用于图片预览，浏览器行为更友好（Content-Disposition: inline）；
//   - 下载走同一端点但不加 inline 参数，后端会强制 attachment。
const attachmentUrl = (id, inline = false) =>
  `/api/ticket/attachment/${id}${inline ? '?inline=1' : ''}`;

// resolveRoleBadge 把后端写入 TicketMessage.role 的数值翻译成前端展示用的徽章。
// 与 common.RoleLabel 对齐：普通用户 / 客服 / 管理员 / 超级管理员。
const resolveRoleBadge = (role, t) => {
  const r = Number(role || 0);
  if (r >= 100) return { text: t('超级管理员'), color: 'red' };
  if (r >= 10) return { text: t('管理员'), color: 'orange' };
  if (r >= 5) return { text: t('客服'), color: 'cyan' };
  return { text: t('用户'), color: 'blue' };
};

const TicketMessageItem = ({ message, isMine, t }) => {
  const badge = resolveRoleBadge(message?.role, t);
  const attachments = Array.isArray(message?.attachments) ? message.attachments : [];
  const images = attachments.filter((a) => isImageMime(a?.mime_type));
  const files = attachments.filter((a) => !isImageMime(a?.mime_type));

  return (
    <div className={`flex ${isMine ? 'justify-end' : 'justify-start'}`}>
      <div
        className='w-full md:max-w-[78%] rounded-2xl px-4 py-3'
        style={{
          background: isMine
            ? 'var(--semi-color-primary-light-default)'
            : 'var(--semi-color-fill-0)',
          border: `1px solid ${
            isMine
              ? 'var(--semi-color-primary-light-hover)'
              : 'var(--semi-color-border)'
          }`,
        }}
      >
        <div className='flex items-start justify-between gap-3 mb-2'>
          <Space spacing={6} wrap>
            <Text strong>{message?.username || t('未知用户')}</Text>
            <Tag color={badge.color} shape='circle' size='small'>
              {badge.text}
            </Tag>
          </Space>
          <Text type='tertiary' size='small'>
            {timestamp2string(message?.created_time || 0)}
          </Text>
        </div>
        {message?.content ? (
          <Paragraph
            className='!mb-0'
            style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}
          >
            {message.content}
          </Paragraph>
        ) : null}

        {images.length > 0 && (
          <div className='mt-2 flex flex-wrap gap-2'>
            <ImagePreview>
              {images.map((img) => (
                <Image
                  key={img.id}
                  width={120}
                  height={120}
                  style={{ objectFit: 'cover', borderRadius: 8 }}
                  src={attachmentUrl(img.id, true)}
                  alt={img.file_name}
                  preview={{
                    src: attachmentUrl(img.id, true),
                  }}
                />
              ))}
            </ImagePreview>
          </div>
        )}

        {files.length > 0 && (
          <div className='mt-2 flex flex-col gap-1'>
            {files.map((f) => (
              <a
                key={f.id}
                href={attachmentUrl(f.id)}
                target='_blank'
                rel='noreferrer'
                className='inline-flex items-center gap-2 no-underline'
                style={{ color: 'var(--semi-color-primary)' }}
              >
                <IconFile />
                <span>{f.file_name}</span>
                <Text type='tertiary' size='small'>
                  ({humanSize(f.size)})
                </Text>
              </a>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default TicketMessageItem;
