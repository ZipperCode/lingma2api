import { test, expect } from '@playwright/test';

const BOOTSTRAP_RESPONSE = {
  id: 'sess-test',
  status: 'awaiting_callback_url',
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

const STATUS_COMPLETED = {
  ...BOOTSTRAP_RESPONSE,
  status: 'completed',
  phase: 'saving',
};

test.describe('Account bootstrap remote_callback flow', () => {
  test('shows auth_url and callback input after starting', async ({ page }) => {
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
    await page.click('button:has-text("浏览器登录")');

    await expect(page.getByText(BOOTSTRAP_RESPONSE.auth_url)).toBeVisible();
    await expect(page.getByPlaceholder(/127.0.0.1:37510/)).toBeVisible();
    await expect(page.getByRole('button', { name: /取消/ })).toBeVisible();
  });

  test('submit callback button calls submit endpoint', async ({ page }) => {
    let submittedPayload: { id?: string; callback_url?: string } | undefined;
    await page.route('**/admin/account', route =>
      route.fulfill({ json: { credential: {}, status: {}, token_stats: {} } }));
    await page.route('**/admin/account/bootstrap', route => {
      if (route.request().method() === 'POST') {
        return route.fulfill({ json: BOOTSTRAP_RESPONSE });
      }
      if (route.request().method() === 'DELETE') {
        return route.fulfill({ json: { status: 'cancelled' } });
      }
      return route.continue();
    });
    await page.route('**/admin/account/bootstrap/submit', route => {
      const raw = route.request().postData() || '{}';
      submittedPayload = JSON.parse(raw) as { id?: string; callback_url?: string };
      return route.fulfill({ json: STATUS_COMPLETED });
    });
    await page.route('**/admin/account/bootstrap/status*', route =>
      route.fulfill({ json: BOOTSTRAP_RESPONSE }));

    await page.goto('/');
    await page.click('a:has-text("账号")');
    await page.click('button:has-text("浏览器登录")');
    await page.getByPlaceholder(/127.0.0.1:37510/).fill('http://127.0.0.1:37510/auth/callback?auth=a&token=b');
    await page.click('button:has-text("提交回调链接")');

    await expect.poll(() => submittedPayload?.id).toBe('sess-test');
    await expect.poll(() => submittedPayload?.callback_url).toContain('127.0.0.1:37510');
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
    await page.click('button:has-text("浏览器登录")');
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
    await page.click('button:has-text("浏览器登录")');

    await expect(page.getByText(/timeout/i)).toBeVisible();
    await expect(page.getByText(/5 分钟内未完成/)).toBeVisible();
  });

  test('browser login button submits remote_callback method', async ({ page }) => {
    let submittedMethod: string | undefined;
    await page.route('**/admin/account', route =>
      route.fulfill({ json: { credential: {}, status: {}, token_stats: {} } }));
    await page.route('**/admin/account/bootstrap', route => {
      if (route.request().method() === 'POST') {
        const raw = route.request().postData() || '{}';
        submittedMethod = (JSON.parse(raw) as { method?: string }).method;
        return route.fulfill({ json: BOOTSTRAP_RESPONSE });
      }
      return route.continue();
    });
    await page.route('**/admin/account/bootstrap/status*', route =>
      route.fulfill({ json: BOOTSTRAP_RESPONSE }));

    await page.goto('/');
    await page.click('a:has-text("账号")');
    await page.click('button:has-text("浏览器登录")');

    await expect.poll(() => submittedMethod).toBe('remote_callback');
    await expect(page.getByText(/复制地址栏里的完整回调链接/)).toBeVisible();
  });
});
