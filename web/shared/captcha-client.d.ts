// Structural boundary: the two themes can resolve different Axios versions.
interface CaptchaHttpClient {
  defaults: { withCredentials?: boolean };
  interceptors: {
    response: {
      use(
        onFulfilled: (response: any) => any,
        onRejected?: (error: any) => any,
      ): number;
    };
  };
  post(url: string, data?: any, config?: any): Promise<any>;
  request(config: any): Promise<any>;
}
export function installCaptchaInterceptor(
  api: CaptchaHttpClient,
  t: (key: string) => string,
): number;
