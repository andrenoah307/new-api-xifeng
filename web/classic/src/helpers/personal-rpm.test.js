import assert from 'node:assert/strict';
import { describe, test } from 'node:test';

import {
  installVisibleTopRefresh,
  normalizePersonalRPMItems,
  personalRPMDisplayState,
  PERSONAL_RPM_REFRESH_INTERVAL,
} from './personal-rpm.js';

describe('personal RPM presentation contract', () => {
  test('sorts by RPM, breaks ties by model, and drops zero values', () => {
    assert.deepEqual(
      normalizePersonalRPMItems([
        { model: 'z', rpm: 2 },
        { model: 'a', rpm: 2 },
        { model: 'zero', rpm: 0 },
        { model: 'one', rpm: 1 },
      ]),
      [
        { model: 'a', rpm: 2 },
        { model: 'z', rpm: 2 },
        { model: 'one', rpm: 1 },
      ],
    );
  });

  test('distinguishes empty from unavailable', () => {
    assert.equal(personalRPMDisplayState('empty', []), 'empty');
    assert.equal(personalRPMDisplayState('available', []), 'empty');
    assert.equal(
      personalRPMDisplayState('overflow', [{ model: 'hidden', rpm: 1 }]),
      'unavailable',
    );
  });

  test('refreshes only while visible and cleans up the interval', () => {
    let refreshes = 0;
    let intervalCallback;
    let removed = false;
    const documentRef = {
      visibilityState: 'hidden',
      addEventListener(_name, callback) {
        this.callback = callback;
      },
      removeEventListener(_name, callback) {
        removed = this.callback === callback;
      },
    };
    const windowRef = {
      setInterval(callback, delay) {
        assert.equal(delay, PERSONAL_RPM_REFRESH_INTERVAL);
        intervalCallback = callback;
        return 1;
      },
      clearInterval(id) {
        assert.equal(id, 1);
      },
    };
    const cleanup = installVisibleTopRefresh({
      documentRef,
      windowRef,
      refresh: () => {
        refreshes += 1;
      },
    });

    intervalCallback();
    assert.equal(refreshes, 0);
    documentRef.visibilityState = 'visible';
    intervalCallback();
    documentRef.callback();
    assert.equal(refreshes, 2);
    cleanup();
    assert.equal(removed, true);
  });
});
