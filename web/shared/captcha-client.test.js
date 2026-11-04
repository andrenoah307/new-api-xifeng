import { describe, test, expect } from 'bun:test';
import axios from 'axios';
import { installCaptchaInterceptor } from './captcha-client.js';

describe('captcha request replay', () => {
  test('preserves multipart attachments when replaying a blocked upload', async () => {
    const bodies = [];
    const api = axios.create({
      adapter: async (config) => {
        let data;
        if (config.url === '/api/captcha/verify')
          data = { success: true, data: { proof: 'c'.repeat(64) } };
        else {
          bodies.push(config.data);
          data =
            bodies.length === 1
              ? { success: false, code: 'captcha_required', data: {} }
              : { success: true };
        }
        return { data, config, status: 200, statusText: 'OK', headers: {} };
      },
    });
    installCaptchaInterceptor(
      api,
      (key) => key,
      async () => ({ ticket: 'ticket' }),
    );
    const form = new FormData();
    form.append('file', new File(['attachment body'], 'test.txt'));
    await api.post('/api/ticket/attachment', form);
    expect(bodies).toHaveLength(2);
    expect(bodies[1]).toBe(bodies[0]);
    expect(await bodies[1].get('file').text()).toBe('attachment body');
  });
  test('replays only the blocked operation with the same body and a one-use proof', async () => {
    const requests = [];
    const api = axios.create({
      adapter: async (config) => {
        requests.push(config);
        const data =
          config.url === '/api/captcha/verify'
            ? { success: true, data: { proof: 'a'.repeat(64) } }
            : config.headers['X-Captcha-Proof']
              ? { success: true }
              : {
                  success: false,
                  code: 'captcha_required',
                  data: { id: 'challenge', provider: 'tencent' },
                };
        return { data, config, status: 200, statusText: 'OK', headers: {} };
      },
    });
    installCaptchaInterceptor(
      api,
      (key) => key,
      async () => ({ ticket: 'verified', randstr: 'nonce' }),
    );
    const result = await api.post('/api/user/pay', {
      amount: 10,
      discount_code: 'code',
    });
    expect(result.data.success).toBe(true);
    expect(requests.map((r) => r.url)).toEqual([
      '/api/user/pay',
      '/api/captcha/verify',
      '/api/user/pay',
    ]);
    expect(requests[2].data).toBe(requests[0].data);
    expect(requests[2].headers['X-Captcha-Proof']).toBe('a'.repeat(64));
  });

  test('does not retry business rejection, unavailable service, or cancellation', async () => {
    for (const mode of [
      'business',
      'unavailable',
      'cancel',
      'repeated-challenge',
    ]) {
      let calls = 0;
      const api = axios.create({
        adapter: async (config) => {
          calls++;
          if (mode === 'unavailable') throw new Error('503');
          const data =
            config.url === '/api/captcha/verify'
              ? { success: true, data: { proof: 'b'.repeat(64) } }
              : {
                  success: false,
                  code: mode === 'business' ? 'rejected' : 'captcha_required',
                  data: {},
                };
          return { data, config, status: 200, statusText: 'OK', headers: {} };
        },
      });
      installCaptchaInterceptor(
        api,
        (key) => key,
        async () => {
          if (mode === 'cancel') throw new Error('cancel');
          return { ticket: 'ticket' };
        },
      );
      if (mode === 'business') await api.post('/api/action', {});
      else await expect(api.post('/api/action', {})).rejects.toThrow();
      expect(calls).toBe(mode === 'repeated-challenge' ? 3 : 1);
    }
  });
});
