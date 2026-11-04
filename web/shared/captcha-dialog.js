const scripts = new Map();

function loadScript(src) {
  if (scripts.has(src)) return scripts.get(src);
  const pending = new Promise((resolve, reject) => {
    const script = document.createElement('script');
    const timer = setTimeout(() => {
      script.remove();
      reject(new Error('script timeout'));
    }, 15000);
    script.src = src;
    script.async = true;
    script.onload = () => {
      clearTimeout(timer);
      resolve();
    };
    script.onerror = () => {
      clearTimeout(timer);
      script.remove();
      reject(new Error('script failed'));
    };
    document.head.append(script);
  }).catch((error) => {
    scripts.delete(src);
    throw error;
  });
  scripts.set(src, pending);
  return pending;
}

export async function solveCaptcha(challenge, t, signal) {
  const tencent = challenge.provider === 'tencent';
  if (!tencent && challenge.provider !== 'turnstile')
    throw new Error(t('Security verification failed. Please try again.'));
  try {
    await loadScript(
      tencent
        ? 'https://turing.captcha.qcloud.com/TJCaptcha.js'
        : 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit',
    );
  } catch {
    throw new Error(t('Security verification failed. Please try again.'));
  }
  if (signal?.aborted) throw new Error(t('Verification cancelled'));
  return new Promise((resolve, reject) => {
    let widget;
    let instance;
    let finished = false;
    const previousFocus = document.activeElement;
    const panel = document.createElement('dialog');
    panel.style.cssText =
      'position:fixed;inset:0;margin:auto;padding:20px;border:1px solid #aaa;border-radius:8px;width:352px;max-width:calc(100vw - 24px);background:Canvas;color:CanvasText;z-index:2147483000;';
    const title = document.createElement('h2');
    title.id = 'captcha-dialog-title';
    title.textContent = t('Security verification');
    title.style.cssText = 'font:600 16px system-ui;margin:0 0 16px;';
    panel.setAttribute('aria-labelledby', title.id);
    const mount = document.createElement('div');
    mount.style.cssText = 'min-height:65px;width:100%;';
    const cancel = document.createElement('button');
    cancel.type = 'button';
    cancel.textContent = t('Cancel');
    cancel.style.cssText =
      'display:block;margin:16px 0 0 auto;padding:8px 16px;font:14px system-ui;cursor:pointer;';
    const finish = (answer, error) => {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
      signal?.removeEventListener('abort', abort);
      if (widget !== undefined) window.turnstile?.remove(widget);
      instance?.destroy?.();
      panel.remove();
      previousFocus?.focus?.();
      if (error) reject(error);
      else resolve(answer);
    };
    const abort = () => finish(null, new Error(t('Verification cancelled')));
    const fail = () =>
      finish(
        null,
        new Error(t('Security verification failed. Please try again.')),
      );
    const timer = setTimeout(fail, 110000);
    cancel.onclick = abort;
    panel.oncancel = (event) => {
      event.preventDefault();
      abort();
    };
    signal?.addEventListener('abort', abort, { once: true });
    try {
      if (tencent) {
        // Tencent renders its own accessible modal. A native top-layer dialog
        // would block that SDK overlay, so only Turnstile uses our dialog.
        instance = new window.TencentCaptcha(
          challenge.app_id,
          (result) => {
            if (result.ret === 2) return abort();
            if (
              result.ret !== 0 ||
              !result.ticket ||
              result.ticket.startsWith('trerror')
            )
              return fail();
            finish({ ticket: result.ticket, randstr: result.randstr });
          },
          challenge.aid_encrypted
            ? { aidEncrypted: challenge.aid_encrypted, aidEncryptedType: 'gcm' }
            : {},
        );
        instance.show();
      } else {
        panel.append(title, mount, cancel);
        document.body.append(panel);
        panel.showModal();
        widget = window.turnstile.render(mount, {
          sitekey: challenge.site_key,
          size: 'flexible',
          callback: (ticket) => finish({ ticket }),
          'error-callback': fail,
          'expired-callback': fail,
          'timeout-callback': fail,
        });
      }
    } catch {
      fail();
    }
  });
}
