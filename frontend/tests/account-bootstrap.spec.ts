import { test, expect } from '@playwright/test';

const BOOTSTRAP_RESPONSE = {
  id: 'sess-test',
  status: 'awaiting_callback',
  method: 'remote_callback',
  auth_url: 'https://account.alibabacloud.com/login/login.htm?fake=1',
  started_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
};

const STATUS_ERROR = {
  ...BOOTSTRAP_RESPONSE,
  status: 'error',
  error: 'timeout: user did not complete login within 5m',
};

test.describe('Account bootstrap remote_callback flow', () => {
  test('shows auth_url and cancel button after starting', async ({ page }) => {
    await page.route('**/admin/account', route =>
      route.fulfill({ json: { credential: {}, status: { loaded: false }, token_stats: {} } }));
    await page.route('**/admin/account/bootstrap', route => {
      if (route.request().method() === 'POST') {
        return route.fulfill({ json: BOOTSTRAP_RESPONSE });
      }
      return route.continue();
    });
    await page.route('**/admin/account/bootstrap/status*', route =>
      route.fulfill({ json: BOOTSTRAP_RESPONSE }));

    await page.goto('/');
    await page.click('a:has-text("账号")');
    await page.click('button:has-text("远程登录")');

    await expect(page.getByText(BOOTSTRAP_RESPONSE.auth_url)).toBeVisible();
    await expect(page.getByRole('button', { name: /取消/ })).toBeVisible();
  });

  test('cancel button calls DELETE endpoint', async ({ page }) => {
    let deleteCalled = false;
    await page.route('**/admin/account', route =>
      route.fulfill({ json: { credential: {}, status: {}, token_stats: {} } }));
    await page.route('**/admin/account/bootstrap', route => {
      if (route.request().method() === 'POST') {
        return route.fulfill({ json: BOOTSTRAP_RESPONSE });
      }
      if (route.request().method() === 'DELETE') {
        deleteCalled = true;
        return route.fulfill({ json: { status: 'cancelled' } });
      }
      return route.continue();
    });
    await page.route('**/admin/account/bootstrap/status*', route =>
      route.fulfill({ json: BOOTSTRAP_RESPONSE }));

    await page.goto('/');
    await page.click('a:has-text("账号")');
    await page.click('button:has-text("远程登录")');
    await page.click('button:has-text("取消")');

    await expect.poll(() => deleteCalled).toBe(true);
  });

  test('shows error message on timeout', async ({ page }) => {
    await page.route('**/admin/account', route =>
      route.fulfill({ json: { credential: {}, status: {}, token_stats: {} } }));
    await page.route('**/admin/account/bootstrap', route =>
      route.fulfill({ json: BOOTSTRAP_RESPONSE }));
    await page.route('**/admin/account/bootstrap/status*', route =>
      route.fulfill({ json: STATUS_ERROR }));

    await page.goto('/');
    await page.click('a:has-text("账号")');
    await page.click('button:has-text("远程登录")');

    await expect(page.getByText(/timeout/i)).toBeVisible();
    await expect(page.getByText(/5 分钟内未完成/)).toBeVisible();
  });

  test.describe('three-button method dispatch', () => {
    const cases: Array<{ label: string; method: 'oauth' | 'ws' | 'remote_callback' }> = [
      { label: 'OAuth 登录', method: 'oauth' },
      { label: '本地灵码', method: 'ws' },
      { label: '远程登录', method: 'remote_callback' },
    ];

    for (const c of cases) {
      test(`button "${c.label}" submits method=${c.method}`, async ({ page }) => {
        let submittedMethod: string | undefined;
        await page.route('**/admin/account', route =>
          route.fulfill({ json: { credential: {}, status: {}, token_stats: {} } }));
        await page.route('**/admin/account/bootstrap', route => {
          if (route.request().method() === 'POST') {
            const raw = route.request().postData() || '{}';
            try {
              submittedMethod = (JSON.parse(raw) as { method?: string }).method;
            } catch {
              submittedMethod = undefined;
            }
            return route.fulfill({ json: { ...BOOTSTRAP_RESPONSE, method: c.method } });
          }
          return route.continue();
        });
        await page.route('**/admin/account/bootstrap/status*', route =>
          route.fulfill({ json: { ...BOOTSTRAP_RESPONSE, method: c.method } }));

        await page.goto('/');
        await page.click('a:has-text("账号")');
        await page.click(`button:has-text("${c.label}")`);

        await expect.poll(() => submittedMethod).toBe(c.method);
      });
    }
  });
});
