export const captchaScenes: Record<string, string>;
export const captchaFields: Record<string, string>;
export const captchaSecretFields: string[];
export interface CaptchaConfig {
  enabled: boolean;
  scenes: Record<string, string>;
  turnstile_site_key: string;
  turnstile_secret: string;
  tencent_app_id: string;
  tencent_app_secret: string;
  tencent_secret_id: string;
  tencent_secret_key: string;
  tencent_app_id_auth: boolean;
}
export interface CaptchaSettingsData {
  config: CaptchaConfig;
  configured: Record<string, boolean>;
  revision: string;
  scenes: string[];
}
