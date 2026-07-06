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

import React, { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  API,
  getLogo,
  showError,
  showInfo,
  showSuccess,
  updateAPI,
  getSystemName,
  getOAuthProviderIcon,
  setUserData,
  onDiscordOAuthClicked,
  onCustomOAuthClicked,
} from '../../helpers';
import {
  getReadStatus as getLegalReadStatus,
  markRead as markLegalRead,
} from '../../helpers/legalConsentStorage';
import Turnstile from 'react-turnstile';
import {
  Button,
  Card,
  Checkbox,
  Divider,
  Form,
  Icon,
  Input,
  Modal,
} from '@douyinfe/semi-ui';
import Title from '@douyinfe/semi-ui/lib/es/typography/title';
import Text from '@douyinfe/semi-ui/lib/es/typography/text';
import {
  IconGithubLogo,
  IconMail,
  IconUser,
  IconLock,
  IconKey,
} from '@douyinfe/semi-icons';
import {
  onGitHubOAuthClicked,
  onLinuxDOOAuthClicked,
  onOIDCClicked,
} from '../../helpers';
import OIDCIcon from '../common/logo/OIDCIcon';
import LinuxDoIcon from '../common/logo/LinuxDoIcon';
import WeChatIcon from '../common/logo/WeChatIcon';
import TelegramLoginButton from 'react-telegram-login/src';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { useTranslation } from 'react-i18next';
import { SiDiscord } from 'react-icons/si';

const RegisterForm = () => {
  let navigate = useNavigate();
  const { t } = useTranslation();
  const githubButtonTextKeyByState = {
    idle: '使用 GitHub 继续',
    redirecting: '正在跳转 GitHub...',
    timeout: '请求超时，请刷新页面后重新发起 GitHub 登录',
  };
  const [inputs, setInputs] = useState({
    username: '',
    password: '',
    password2: '',
    email: '',
    verification_code: '',
    wechat_verification_code: '',
    invitation_code: '',
  });
  const { username, password, password2 } = inputs;
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [showWeChatLoginModal, setShowWeChatLoginModal] = useState(false);
  const [showEmailRegister, setShowEmailRegister] = useState(false);
  const [wechatLoading, setWechatLoading] = useState(false);
  const [githubLoading, setGithubLoading] = useState(false);
  const [discordLoading, setDiscordLoading] = useState(false);
  const [oidcLoading, setOidcLoading] = useState(false);
  const [linuxdoLoading, setLinuxdoLoading] = useState(false);
  const [emailRegisterLoading, setEmailRegisterLoading] = useState(false);
  const [registerLoading, setRegisterLoading] = useState(false);
  const [verificationCodeLoading, setVerificationCodeLoading] = useState(false);
  const [otherRegisterOptionsLoading, setOtherRegisterOptionsLoading] =
    useState(false);
  const [wechatCodeSubmitLoading, setWechatCodeSubmitLoading] = useState(false);
  const [customOAuthLoading, setCustomOAuthLoading] = useState({});
  const [disableButton, setDisableButton] = useState(false);
  const [countdown, setCountdown] = useState(30);
  const [hasUserAgreement, setHasUserAgreement] = useState(false);
  const [hasPrivacyPolicy, setHasPrivacyPolicy] = useState(false);
  const [legalDocHash, setLegalDocHash] = useState('');
  const [legalDocContent, setLegalDocContent] = useState('');
  const [legalAgreed, setLegalAgreed] = useState({
    'user-agreement': false,
    'privacy-policy': false,
    'terms-of-service': false,
  });
  const [legalModalDoc, setLegalModalDoc] = useState(null);
  const consentRequired = hasUserAgreement || hasPrivacyPolicy;
  const allAgreedToTerms = !consentRequired
    ? true
    : Boolean(legalDocHash) &&
      legalAgreed['user-agreement'] &&
      legalAgreed['privacy-policy'] &&
      legalAgreed['terms-of-service'];
  const [githubButtonState, setGithubButtonState] = useState('idle');
  const [githubButtonDisabled, setGithubButtonDisabled] = useState(false);
  const githubTimeoutRef = useRef(null);
  const githubButtonText = t(githubButtonTextKeyByState[githubButtonState]);

  const logo = getLogo();
  const systemName = getSystemName();
  const urlSearchParams = useMemo(
    () => new URLSearchParams(window.location.search),
    [],
  );
  const affCodeFromUrl = urlSearchParams.get('aff') || '';
  const invitationCodeFromUrl =
    urlSearchParams.get('code') || urlSearchParams.get('invitation_code') || '';

  const status = useMemo(() => {
    if (statusState?.status) return statusState.status;
    const savedStatus = localStorage.getItem('status');
    if (!savedStatus) return {};
    try {
      return JSON.parse(savedStatus) || {};
    } catch (err) {
      return {};
    }
  }, [statusState?.status]);
  const hasCustomOAuthProviders =
    (status.custom_oauth_providers || []).length > 0;
  const hasOAuthRegisterOptions = Boolean(
    status.github_oauth ||
      status.discord_oauth ||
      status.oidc_enabled ||
      status.wechat_login ||
      status.linuxdo_oauth ||
      status.telegram_oauth ||
      hasCustomOAuthProviders,
  );

  const [showEmailVerification, setShowEmailVerification] = useState(false);
  const invitationCodeValue = inputs.invitation_code.trim();
  const showPasswordInvitationInput = Boolean(
    status.invitation_code_enabled || invitationCodeValue,
  );
  const showOAuthInvitationInput = Boolean(
    status.invitation_code_oauth_required || invitationCodeValue,
  );

  useEffect(() => {
    if (affCodeFromUrl) {
      localStorage.setItem('aff', affCodeFromUrl);
    }
  }, [affCodeFromUrl]);

  useEffect(() => {
    if (!invitationCodeFromUrl) {
      return;
    }
    setInputs((prev) => {
      if (prev.invitation_code) {
        return prev;
      }
      return {
        ...prev,
        invitation_code: invitationCodeFromUrl,
      };
    });
  }, [invitationCodeFromUrl]);

  useEffect(() => {
    setShowEmailVerification(!!status?.email_verification);
    if (status?.turnstile_check) {
      setTurnstileEnabled(true);
      setTurnstileSiteKey(status.turnstile_site_key);
    }

    // 从 status 获取用户协议和隐私政策的启用状态
    setHasUserAgreement(status?.user_agreement_enabled || false);
    setHasPrivacyPolicy(status?.privacy_policy_enabled || false);
  }, [status]);

  useEffect(() => {
    if (!consentRequired) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await API.get('/api/user-agreement');
        if (cancelled) return;
        const data = res?.data || {};
        const content = data.data || '';
        const hash = data.hash || '';
        setLegalDocContent(content);
        setLegalDocHash(hash);
        if (hash) {
          setLegalAgreed({
            'user-agreement': getLegalReadStatus('user-agreement', hash),
            'privacy-policy': getLegalReadStatus('privacy-policy', hash),
            'terms-of-service': getLegalReadStatus('terms-of-service', hash),
          });
        } else {
          setLegalAgreed({
            'user-agreement': false,
            'privacy-policy': false,
            'terms-of-service': false,
          });
        }
      } catch {
        // ignore
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [consentRequired]);

  const showLegalConsentError = () =>
    showInfo(t('请先阅读并同意用户协议、隐私政策和服务条款'));

  const handleLegalCheckboxToggle = (docKey, nextValue) => {
    if (nextValue) {
      setLegalModalDoc(docKey);
    } else {
      setLegalAgreed((prev) => ({ ...prev, [docKey]: false }));
    }
  };

  const handleLegalModalConfirm = () => {
    if (!legalModalDoc || !legalDocHash) {
      setLegalModalDoc(null);
      return;
    }
    markLegalRead(legalModalDoc, legalDocHash);
    setLegalAgreed((prev) => ({ ...prev, [legalModalDoc]: true }));
    setLegalModalDoc(null);
  };

  const legalModalTitleMap = {
    'user-agreement': t('用户协议'),
    'privacy-policy': t('隐私政策'),
    'terms-of-service': t('服务条款'),
  };

  const renderLegalConsentBlock = () => {
    if (!consentRequired) return null;
    const items = [
      { key: 'user-agreement', label: t('用户协议') },
      { key: 'privacy-policy', label: t('隐私政策') },
      { key: 'terms-of-service', label: t('服务条款') },
    ];
    return (
      <div className='space-y-2'>
        {items.map((item) => (
          <Checkbox
            key={item.key}
            checked={legalAgreed[item.key]}
            disabled={!legalDocHash}
            onChange={(e) =>
              handleLegalCheckboxToggle(item.key, e.target.checked)
            }
          >
            <Text size='small' className='text-gray-600'>
              {t('我已阅读并同意')}
              <a
                href='#'
                onClick={(ev) => {
                  ev.preventDefault();
                  setLegalModalDoc(item.key);
                }}
                className='text-blue-600 hover:text-blue-800 mx-1'
              >
                《{item.label}》
              </a>
            </Text>
          </Checkbox>
        ))}
      </div>
    );
  };

  const renderLegalConsentModal = () => {
    if (!consentRequired) return null;
    return (
      <Modal
        title={legalModalDoc ? legalModalTitleMap[legalModalDoc] : ''}
        visible={legalModalDoc !== null}
        maskClosable={true}
        onOk={handleLegalModalConfirm}
        onCancel={() => setLegalModalDoc(null)}
        okText={t('我已阅读')}
        cancelText={t('关闭')}
        centered={true}
        width={640}
        style={{ maxWidth: '92vw' }}
        bodyStyle={{
          maxHeight: 'calc(80vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
        getPopupContainer={() => document.body}
        okButtonProps={{ disabled: !legalDocHash }}
      >
        {legalDocContent ? (
          <div
            className='text-sm whitespace-pre-wrap break-words'
            dangerouslySetInnerHTML={{ __html: legalDocContent }}
          />
        ) : (
          <Text type='secondary'>{t('文档尚未配置')}</Text>
        )}
      </Modal>
    );
  };

  useEffect(() => {
    let countdownInterval = null;
    if (disableButton && countdown > 0) {
      countdownInterval = setInterval(() => {
        setCountdown(countdown - 1);
      }, 1000);
    } else if (countdown === 0) {
      setDisableButton(false);
      setCountdown(30);
    }
    return () => clearInterval(countdownInterval); // Clean up on unmount
  }, [disableButton, countdown]);

  useEffect(() => {
    return () => {
      if (githubTimeoutRef.current) {
        clearTimeout(githubTimeoutRef.current);
      }
    };
  }, []);

  const onWeChatLoginClicked = () => {
    if (!allAgreedToTerms) {
      showLegalConsentError();
      return;
    }
    if (!ensureOAuthInvitationCode()) {
      return;
    }
    setWechatLoading(true);
    setShowWeChatLoginModal(true);
    setWechatLoading(false);
  };

  const onSubmitWeChatVerificationCode = async () => {
    if (!ensureOAuthInvitationCode()) {
      return;
    }
    if (turnstileEnabled && turnstileToken === '') {
      showInfo('请稍后几秒重试，Turnstile 正在检查用户环境！');
      return;
    }
    setWechatCodeSubmitLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('code', inputs.wechat_verification_code);
      if (invitationCodeValue) {
        params.set('invitation_code', invitationCodeValue);
      }
      const res = await API.get(`/api/oauth/wechat?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
        localStorage.setItem('user', JSON.stringify(data));
        setUserData(data);
        updateAPI();
        navigate('/');
        showSuccess('登录成功！');
        setShowWeChatLoginModal(false);
      } else {
        showError(message);
      }
    } catch (error) {
      showError('登录失败，请重试');
    } finally {
      setWechatCodeSubmitLoading(false);
    }
  };

  function handleChange(name, value) {
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  }

  const ensureOAuthInvitationCode = () => {
    if (!status.invitation_code_oauth_required || invitationCodeValue) {
      return true;
    }
    showInfo('请输入邀请码');
    return false;
  };

  const getOAuthOptions = () => {
    const options = {
      shouldLogout: true,
    };
    if (invitationCodeValue) {
      options.invitationCode = invitationCodeValue;
    }
    return options;
  };

  async function handleSubmit(e) {
    if (!allAgreedToTerms) {
      showLegalConsentError();
      return;
    }
    if (password.length < 8) {
      showInfo('密码长度不得小于 8 位！');
      return;
    }
    if (password !== password2) {
      showInfo('两次输入的密码不一致');
      return;
    }
    if (username && password) {
      if (status.invitation_code_enabled && !invitationCodeValue) {
        showInfo('请输入邀请码');
        return;
      }
      if (turnstileEnabled && turnstileToken === '') {
        showInfo('请稍后几秒重试，Turnstile 正在检查用户环境！');
        return;
      }
      setRegisterLoading(true);
      try {
        const affCode = affCodeFromUrl || localStorage.getItem('aff') || '';
        const payload = {
          ...inputs,
          aff_code: affCode,
          invitation_code: invitationCodeValue,
        };
        const res = await API.post(
          `/api/user/register?turnstile=${turnstileToken}`,
          payload,
        );
        const { success, message } = res.data;
        if (success) {
          navigate('/login');
          showSuccess('注册成功！');
        } else {
          showError(message);
        }
      } catch (error) {
        showError('注册失败，请重试');
      } finally {
        setRegisterLoading(false);
      }
    }
  }

  const sendVerificationCode = async () => {
    if (inputs.email === '') return;
    if (turnstileEnabled && turnstileToken === '') {
      showInfo('请稍后几秒重试，Turnstile 正在检查用户环境！');
      return;
    }
    setVerificationCodeLoading(true);
    try {
      const res = await API.get(
        `/api/verification?email=${encodeURIComponent(inputs.email)}&turnstile=${turnstileToken}`,
      );
      const { success, message } = res.data;
      if (success) {
        showSuccess('验证码发送成功，请检查你的邮箱！');
        setDisableButton(true); // 发送成功后禁用按钮，开始倒计时
      } else {
        showError(message);
      }
    } catch (error) {
      showError('发送验证码失败，请重试');
    } finally {
      setVerificationCodeLoading(false);
    }
  };

  const handleGitHubClick = () => {
    if (!allAgreedToTerms) {
      showLegalConsentError();
      return;
    }
    if (!ensureOAuthInvitationCode()) {
      return;
    }
    if (githubButtonDisabled) {
      return;
    }
    setGithubLoading(true);
    setGithubButtonDisabled(true);
    setGithubButtonState('redirecting');
    if (githubTimeoutRef.current) {
      clearTimeout(githubTimeoutRef.current);
    }
    githubTimeoutRef.current = setTimeout(() => {
      setGithubLoading(false);
      setGithubButtonState('timeout');
      setGithubButtonDisabled(true);
    }, 20000);
    try {
      onGitHubOAuthClicked(status.github_client_id, getOAuthOptions());
    } finally {
      setTimeout(() => setGithubLoading(false), 3000);
    }
  };

  const handleDiscordClick = () => {
    if (!allAgreedToTerms) {
      showLegalConsentError();
      return;
    }
    if (!ensureOAuthInvitationCode()) {
      return;
    }
    setDiscordLoading(true);
    try {
      onDiscordOAuthClicked(status.discord_client_id, getOAuthOptions());
    } finally {
      setTimeout(() => setDiscordLoading(false), 3000);
    }
  };

  const handleOIDCClick = () => {
    if (!allAgreedToTerms) {
      showLegalConsentError();
      return;
    }
    if (!ensureOAuthInvitationCode()) {
      return;
    }
    setOidcLoading(true);
    try {
      onOIDCClicked(
        status.oidc_authorization_endpoint,
        status.oidc_client_id,
        false,
        getOAuthOptions(),
      );
    } finally {
      setTimeout(() => setOidcLoading(false), 3000);
    }
  };

  const handleLinuxDOClick = () => {
    if (!allAgreedToTerms) {
      showLegalConsentError();
      return;
    }
    if (!ensureOAuthInvitationCode()) {
      return;
    }
    setLinuxdoLoading(true);
    try {
      onLinuxDOOAuthClicked(status.linuxdo_client_id, getOAuthOptions());
    } finally {
      setTimeout(() => setLinuxdoLoading(false), 3000);
    }
  };

  const handleCustomOAuthClick = (provider) => {
    if (!allAgreedToTerms) {
      showLegalConsentError();
      return;
    }
    if (!ensureOAuthInvitationCode()) {
      return;
    }
    setCustomOAuthLoading((prev) => ({ ...prev, [provider.slug]: true }));
    try {
      onCustomOAuthClicked(provider, getOAuthOptions());
    } finally {
      setTimeout(() => {
        setCustomOAuthLoading((prev) => ({ ...prev, [provider.slug]: false }));
      }, 3000);
    }
  };

  const handleEmailRegisterClick = () => {
    setEmailRegisterLoading(true);
    setShowEmailRegister(true);
    setEmailRegisterLoading(false);
  };

  const handleOtherRegisterOptionsClick = () => {
    setOtherRegisterOptionsLoading(true);
    setShowEmailRegister(false);
    setOtherRegisterOptionsLoading(false);
  };

  const onTelegramLoginClicked = async (response) => {
    if (!allAgreedToTerms) {
      showLegalConsentError();
      return;
    }
    const fields = [
      'id',
      'first_name',
      'last_name',
      'username',
      'photo_url',
      'auth_date',
      'hash',
      'lang',
    ];
    const params = {};
    fields.forEach((field) => {
      if (response[field]) {
        params[field] = response[field];
      }
    });
    try {
      const res = await API.get(`/api/oauth/telegram/login`, { params });
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
        localStorage.setItem('user', JSON.stringify(data));
        showSuccess('登录成功！');
        setUserData(data);
        updateAPI();
        navigate('/');
      } else {
        showError(message);
      }
    } catch (error) {
      showError('登录失败，请重试');
    }
  };

  const renderOAuthOptions = () => {
    return (
      <div className='flex flex-col items-center'>
        <div className='w-full max-w-md'>
          <div className='flex items-center justify-center mb-6 gap-2'>
            <img src={logo} alt='Logo' className='h-10 rounded-full' />
            <Title heading={3} className='!text-gray-800'>
              {systemName}
            </Title>
          </div>

          <Card className='border-0 !rounded-2xl overflow-hidden'>
            <div className='flex justify-center pt-6 pb-2'>
              <Title heading={3} className='text-gray-800 dark:text-gray-200'>
                {t('注 册')}
              </Title>
            </div>
            <div className='px-2 py-8'>
              <div className='space-y-3'>
                {showOAuthInvitationInput && (
                  <div className='space-y-2'>
                    <Text>{t('邀请码')}</Text>
                    <Input
                      placeholder={t('请输入邀请码')}
                      value={inputs.invitation_code}
                      onChange={(value) =>
                        handleChange('invitation_code', value)
                      }
                      prefix={<IconKey />}
                    />
                  </div>
                )}

                {status.wechat_login && (
                  <Button
                    theme='outline'
                    className='w-full h-12 flex items-center justify-center !rounded-full border border-gray-200 hover:bg-gray-50 transition-colors'
                    type='tertiary'
                    icon={
                      <Icon svg={<WeChatIcon />} style={{ color: '#07C160' }} />
                    }
                    onClick={onWeChatLoginClicked}
                    loading={wechatLoading}
                  >
                    <span className='ml-3'>{t('使用 微信 继续')}</span>
                  </Button>
                )}

                {status.github_oauth && (
                  <Button
                    theme='outline'
                    className='w-full h-12 flex items-center justify-center !rounded-full border border-gray-200 hover:bg-gray-50 transition-colors'
                    type='tertiary'
                    icon={<IconGithubLogo size='large' />}
                    onClick={handleGitHubClick}
                    loading={githubLoading}
                    disabled={githubButtonDisabled}
                  >
                    <span className='ml-3'>{githubButtonText}</span>
                  </Button>
                )}

                {status.discord_oauth && (
                  <Button
                    theme='outline'
                    className='w-full h-12 flex items-center justify-center !rounded-full border border-gray-200 hover:bg-gray-50 transition-colors'
                    type='tertiary'
                    icon={
                      <SiDiscord
                        style={{
                          color: '#5865F2',
                          width: '20px',
                          height: '20px',
                        }}
                      />
                    }
                    onClick={handleDiscordClick}
                    loading={discordLoading}
                  >
                    <span className='ml-3'>{t('使用 Discord 继续')}</span>
                  </Button>
                )}

                {status.oidc_enabled && (
                  <Button
                    theme='outline'
                    className='w-full h-12 flex items-center justify-center !rounded-full border border-gray-200 hover:bg-gray-50 transition-colors'
                    type='tertiary'
                    icon={<OIDCIcon style={{ color: '#1877F2' }} />}
                    onClick={handleOIDCClick}
                    loading={oidcLoading}
                  >
                    <span className='ml-3'>{t('使用 OIDC 继续')}</span>
                  </Button>
                )}

                {status.linuxdo_oauth && (
                  <Button
                    theme='outline'
                    className='w-full h-12 flex items-center justify-center !rounded-full border border-gray-200 hover:bg-gray-50 transition-colors'
                    type='tertiary'
                    icon={
                      <LinuxDoIcon
                        style={{
                          color: '#E95420',
                          width: '20px',
                          height: '20px',
                        }}
                      />
                    }
                    onClick={handleLinuxDOClick}
                    loading={linuxdoLoading}
                  >
                    <span className='ml-3'>{t('使用 LinuxDO 继续')}</span>
                  </Button>
                )}

                {status.custom_oauth_providers &&
                  status.custom_oauth_providers.map((provider) => (
                    <Button
                      key={provider.slug}
                      theme='outline'
                      className='w-full h-12 flex items-center justify-center !rounded-full border border-gray-200 hover:bg-gray-50 transition-colors'
                      type='tertiary'
                      icon={getOAuthProviderIcon(provider.icon || '', 20)}
                      onClick={() => handleCustomOAuthClick(provider)}
                      loading={customOAuthLoading[provider.slug]}
                    >
                      <span className='ml-3'>
                        {t('使用 {{name}} 继续', { name: provider.name })}
                      </span>
                    </Button>
                  ))}

                {status.telegram_oauth && (
                  <div className='flex justify-center my-2'>
                    <TelegramLoginButton
                      dataOnauth={onTelegramLoginClicked}
                      botName={status.telegram_bot_name}
                    />
                  </div>
                )}

                <Divider margin='12px' align='center'>
                  {t('或')}
                </Divider>

                <Button
                  theme='solid'
                  type='primary'
                  className='w-full h-12 flex items-center justify-center bg-black text-white !rounded-full hover:bg-gray-800 transition-colors'
                  icon={<IconMail size='large' />}
                  onClick={handleEmailRegisterClick}
                  loading={emailRegisterLoading}
                >
                  <span className='ml-3'>{t('使用 用户名 注册')}</span>
                </Button>
              </div>

              <div className='mt-6 text-center text-sm'>
                <Text>
                  {t('已有账户？')}{' '}
                  <Link
                    to='/login'
                    className='text-blue-600 hover:text-blue-800 font-medium'
                  >
                    {t('登录')}
                  </Link>
                </Text>
              </div>
            </div>
          </Card>
        </div>
      </div>
    );
  };

  const renderEmailRegisterForm = () => {
    return (
      <div className='flex flex-col items-center'>
        <div className='w-full max-w-md'>
          <div className='flex items-center justify-center mb-6 gap-2'>
            <img src={logo} alt='Logo' className='h-10 rounded-full' />
            <Title heading={3} className='!text-gray-800'>
              {systemName}
            </Title>
          </div>

          <Card className='border-0 !rounded-2xl overflow-hidden'>
            <div className='flex justify-center pt-6 pb-2'>
              <Title heading={3} className='text-gray-800 dark:text-gray-200'>
                {t('注 册')}
              </Title>
            </div>
            <div className='px-2 py-8'>
              <Form className='space-y-3'>
                <Form.Input
                  field='username'
                  label={t('用户名')}
                  placeholder={t('请输入用户名')}
                  name='username'
                  onChange={(value) => handleChange('username', value)}
                  prefix={<IconUser />}
                />

                <Form.Input
                  field='password'
                  label={t('密码')}
                  placeholder={t('输入密码，最短 8 位，最长 20 位')}
                  name='password'
                  mode='password'
                  onChange={(value) => handleChange('password', value)}
                  prefix={<IconLock />}
                />

                <Form.Input
                  field='password2'
                  label={t('确认密码')}
                  placeholder={t('确认密码')}
                  name='password2'
                  mode='password'
                  onChange={(value) => handleChange('password2', value)}
                  prefix={<IconLock />}
                />

                {showEmailVerification && (
                  <>
                    <Form.Input
                      field='email'
                      label={t('邮箱')}
                      placeholder={t('输入邮箱地址')}
                      name='email'
                      type='email'
                      onChange={(value) => handleChange('email', value)}
                      prefix={<IconMail />}
                      suffix={
                        <Button
                          onClick={sendVerificationCode}
                          loading={verificationCodeLoading}
                          disabled={disableButton || verificationCodeLoading}
                        >
                          {disableButton
                            ? `${t('重新发送')} (${countdown})`
                            : t('获取验证码')}
                        </Button>
                      }
                    />
                    <Form.Input
                      field='verification_code'
                      label={t('验证码')}
                      placeholder={t('输入验证码')}
                      name='verification_code'
                      onChange={(value) =>
                        handleChange('verification_code', value)
                      }
                      prefix={<IconKey />}
                    />
                  </>
                )}

                {showPasswordInvitationInput && (
                  <Form.Input
                    field='invitation_code'
                    label={t('邀请码')}
                    placeholder={t('请输入邀请码')}
                    name='invitation_code'
                    value={inputs.invitation_code}
                    onChange={(value) => handleChange('invitation_code', value)}
                    prefix={<IconKey />}
                  />
                )}

                {(hasUserAgreement || hasPrivacyPolicy) && (
                  <div className='pt-4'>
                    {renderLegalConsentBlock()}
                  </div>
                )}

                <div className='space-y-2 pt-2'>
                  <Button
                    theme='solid'
                    className='w-full !rounded-full'
                    type='primary'
                    htmlType='submit'
                    onClick={handleSubmit}
                    loading={registerLoading}
                    disabled={!allAgreedToTerms}
                  >
                    {t('注册')}
                  </Button>
                </div>
              </Form>

              {hasOAuthRegisterOptions && (
                <>
                  <Divider margin='12px' align='center'>
                    {t('或')}
                  </Divider>

                  <div className='mt-4 text-center'>
                    <Button
                      theme='outline'
                      type='tertiary'
                      className='w-full !rounded-full'
                      onClick={handleOtherRegisterOptionsClick}
                      loading={otherRegisterOptionsLoading}
                    >
                      {t('其他注册选项')}
                    </Button>
                  </div>
                </>
              )}

              <div className='mt-6 text-center text-sm'>
                <Text>
                  {t('已有账户？')}{' '}
                  <Link
                    to='/login'
                    className='text-blue-600 hover:text-blue-800 font-medium'
                  >
                    {t('登录')}
                  </Link>
                </Text>
              </div>
            </div>
          </Card>
        </div>
      </div>
    );
  };

  const renderWeChatLoginModal = () => {
    return (
      <Modal
        title={t('微信扫码登录')}
        visible={showWeChatLoginModal}
        maskClosable={true}
        onOk={onSubmitWeChatVerificationCode}
        onCancel={() => setShowWeChatLoginModal(false)}
        okText={t('登录')}
        centered={true}
        okButtonProps={{
          loading: wechatCodeSubmitLoading,
        }}
      >
        <div className='flex flex-col items-center'>
          <img src={status.wechat_qrcode} alt='微信二维码' className='mb-4' />
        </div>

        <div className='text-center mb-4'>
          <p>
            {t('微信扫码关注公众号，输入「验证码」获取验证码（三分钟内有效）')}
          </p>
        </div>

        <Form>
          <Form.Input
            field='wechat_verification_code'
            placeholder={t('验证码')}
            label={t('验证码')}
            value={inputs.wechat_verification_code}
            onChange={(value) =>
              handleChange('wechat_verification_code', value)
            }
          />
        </Form>
      </Modal>
    );
  };

  return (
    <div className='classic-page-fill relative overflow-hidden bg-gray-100 flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8'>
      {/* 背景模糊晕染球 */}
      <div
        className='blur-ball blur-ball-indigo'
        style={{ top: '-80px', right: '-80px', transform: 'none' }}
      />
      <div
        className='blur-ball blur-ball-teal'
        style={{ top: '50%', left: '-120px' }}
      />
      <div className='w-full max-w-sm mt-[60px]'>
        {showEmailRegister ||
        !hasOAuthRegisterOptions
          ? renderEmailRegisterForm()
          : renderOAuthOptions()}
        {renderWeChatLoginModal()}
        {renderLegalConsentModal()}

        {turnstileEnabled && (
          <div className='flex justify-center mt-6'>
            <Turnstile
              sitekey={turnstileSiteKey}
              onVerify={(token) => {
                setTurnstileToken(token);
              }}
            />
          </div>
        )}
      </div>
    </div>
  );
};

export default RegisterForm;
