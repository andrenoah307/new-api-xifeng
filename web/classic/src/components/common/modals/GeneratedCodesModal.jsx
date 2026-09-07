/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React from 'react';
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Modal,
  Space,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconClose,
  IconCopy,
  IconDownload,
} from '@douyinfe/semi-icons';
import {
  copy,
  downloadTextAsFile,
  showError,
  showSuccess,
} from '../../../helpers';

const { Text } = Typography;

const GeneratedCodesModal = ({
  visible,
  onClose,
  title,
  codes = [],
  filename,
  partial = false,
}) => {
  const { t } = useTranslation();
  const displayedCodes = codes.slice(0, 6);
  const codeText = codes.join('\n');

  const handleDownload = () => {
    try {
      downloadTextAsFile(codeText, `${filename}.txt`);
    } catch (error) {
      showError(error.message || t('下载失败'));
    }
  };

  const handleCopy = async () => {
    if (await copy(codeText)) {
      showSuccess(t('已复制到剪贴板！'));
    } else {
      Modal.error({
        title: t('无法复制到剪贴板，请手动复制'),
        content: codeText,
        size: 'large',
      });
    }
  };

  return (
    <Modal
      title={title}
      visible={visible}
      onCancel={onClose}
      centered
      width={620}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
      footer={
        <div className='flex justify-end'>
          <Space>
            <Button icon={<IconDownload />} onClick={handleDownload}>
              {t('下载')}
            </Button>
            <Button icon={<IconCopy />} onClick={handleCopy}>
              {t('复制全部')}
            </Button>
            <Button
              theme='solid'
              type='primary'
              icon={<IconClose />}
              onClick={onClose}
            >
              {t('关闭')}
            </Button>
          </Space>
        </div>
      }
    >
      <div className='flex flex-col gap-3'>
        {partial && (
          <Banner
            type='warning'
            description={t('创建中途失败，出错前已生成 {{count}} 个码。', {
              count: codes.length,
            })}
            closeIcon={null}
            style={{ marginBottom: 0 }}
          />
        )}
        <Text>
          {t('请立即保存这些码，关闭本对话框后将无法再次查看。')}
        </Text>
        <div
          style={{
            fontFamily: 'monospace',
            whiteSpace: 'pre-wrap',
            overflowWrap: 'anywhere',
            padding: '12px',
            border: '1px solid var(--semi-color-border)',
            borderRadius: '6px',
          }}
        >
          {displayedCodes.map((code, index) => (
            <div key={`${index}-${code}`}>{code}</div>
          ))}
        </div>
        {codes.length > 6 && (
          <Text type='tertiary'>
            {t('已显示 {{shown}} / {{count}}', {
              shown: 6,
              count: codes.length,
            })}
          </Text>
        )}
      </div>
    </Modal>
  );
};

export default GeneratedCodesModal;
