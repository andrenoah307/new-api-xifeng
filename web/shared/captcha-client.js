// Only a server challenge emitted before business execution permits replay.
export function installCaptchaInterceptor(api, t, solve) {
  api.defaults.withCredentials = true;
  let queue = Promise.resolve();
  return api.interceptors.response.use(
    async (response) => {
      if (
        ['captcha_failed', 'captcha_unavailable'].includes(response.data?.code)
      ) {
        response.data.message = t(
          'Security verification failed. Please try again.',
        );
      }
      if (
        response.data?.code !== 'captcha_required' ||
        response.data?.success !== false
      )
        return response;
      const config = response.config;
      if (config._captchaRetried)
        throw new Error(t('Security verification failed. Please try again.'));
      config._captchaRetried = true;
      const run = async () => {
        if (config.signal?.aborted)
          throw new Error(t('Verification cancelled'));
        const challenge = response.data.data;
        const verify =
          solve || (await import('./captcha-dialog.js')).solveCaptcha;
        const answer = await verify(challenge, t, config.signal);
        const verified = await api.post(
          '/api/captcha/verify',
          { id: challenge.id, ...answer },
          { skipBusinessError: true },
        );
        const proof = verified.data?.data?.proof;
        if (
          !verified.data?.success ||
          typeof proof !== 'string' ||
          !/^[a-f0-9]{64}$/.test(proof)
        ) {
          throw new Error(t('Security verification failed. Please try again.'));
        }
        if (config.signal?.aborted)
          throw new Error(t('Verification cancelled'));
        config.headers['X-Captcha-Proof'] = proof;
        return api.request(config);
      };
      const pending = queue.then(run, run);
      queue = pending.catch(() => {});
      return pending;
    },
    (error) => {
      if (
        ['captcha_failed', 'captcha_unavailable'].includes(
          error.response?.data?.code,
        )
      ) {
        error.response.data.message = t(
          'Security verification failed. Please try again.',
        );
      }
      return Promise.reject(error);
    },
  );
}
