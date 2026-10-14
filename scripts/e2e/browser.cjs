// Executed by test-stack-e2e.py with an isolated, pinned Playwright installation.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.ZZFUSE_PLAYWRIGHT_DIR, 'node_modules/playwright'));

async function resetPage(page, url) {
  const initial = await page.context().request.get(url, { maxRedirects: 0 });
  assert.equal(initial.headers()['referrer-policy'], 'no-referrer');
  assert.ok(initial.headers()['cache-control'].includes('no-store'));
  const leakedUrls = [];
  page.on('request', request => {
    const target = new URL(request.url());
    if (target.searchParams.has('token') || target.pathname.startsWith('/reset-password/')) {
      leakedUrls.push(request.resourceType());
    }
  });
  const response = await page.goto(url);
  assert.equal(response.headers()['referrer-policy'], 'no-referrer');
  assert.ok(response.headers()['cache-control'].includes('no-store'));
  await page.getByRole('heading', { name: 'Set New Password', exact: true }).waitFor();
  assert.equal(new URL(page.url()).pathname, '/reset-password');
  assert.equal(new URL(page.url()).hash, '');
  assert.equal(new URL(page.url()).searchParams.has('token'), false);
  assert.deepEqual(leakedUrls, [], 'Reset token entered an HTTP request URL');
  return leakedUrls;
}

async function main() {
  const input = JSON.parse(fs.readFileSync(0, 'utf8'));
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  page.setDefaultTimeout(30_000);
  try {
    if (input.mode === 'register-login') {
      await page.goto(input.url + '/admin');
      await page.waitForURL('**/login');
      await page.goto(input.url + '/register');
      await page.getByLabel('Username', { exact: true }).fill(input.username);
      await page.getByLabel('Email Address', { exact: true }).fill(input.email);
      await page.getByLabel('Password', { exact: true }).fill(input.password);
      const [registered] = await Promise.all([
        page.waitForResponse(response => new URL(response.url()).pathname === '/api/register' &&
                                        response.request().method() === 'POST'),
        page.getByRole('button', { name: 'Register', exact: true }).click(),
      ]);
      assert.equal(registered.status(), 201, 'Frontend registration failed: ' + await registered.text());
      await page.waitForURL(input.url + '/');
      const signedUpSession = await page.evaluate(() => fetch('/api/auth/session').then(response => response.json()));
      assert.equal(signedUpSession.user.email, input.email);
      await context.clearCookies();
      await page.goto(input.url + '/login');
      await page.getByLabel('Email Address', { exact: true }).fill(input.email);
      await page.getByLabel('Password', { exact: true }).fill('wrong-test-password');
      await page.getByRole('button', { name: 'Sign in with Email', exact: true }).click();
      await page.getByRole('alert').waitFor();
      assert.equal(new URL(page.url()).pathname, '/login');
      await page.getByLabel('Password', { exact: true }).fill(input.password);
      await page.getByRole('button', { name: 'Sign in with Email', exact: true }).click();
      await page.waitForURL('**/dashboard');
    } else if (input.mode === 'reset-login') {
      if (input.signedIn) {
        await page.goto(input.url + '/login');
        await page.getByLabel('Email Address', { exact: true }).fill(input.email);
        await page.getByLabel('Password', { exact: true }).fill(input.oldPassword);
        await page.getByRole('button', { name: 'Sign in with Email', exact: true }).click();
        await page.waitForURL('**/dashboard');
      }
      const resetReferers = [];
      page.on('request', request => {
        if ((request.headers().referer || '').includes('/reset-password')) resetReferers.push(request.resourceType());
      });
      const resetRequests = await resetPage(page, input.resetUrl);
      await page.getByLabel('New Password', { exact: true }).fill(input.password);
      await page.getByLabel('Confirm Password', { exact: true }).fill(input.password);
      await page.getByRole('button', { name: 'Set New Password', exact: true }).click();
      await page.waitForURL('**/login');
      assert.deepEqual(resetRequests, [], 'Reset submission exposed its token in an HTTP request URL');
      const loggedOutSession = await page.evaluate(() => fetch('/api/auth/session').then(response => response.json()));
      assert.ok(!loggedOutSession.user, 'Reset completion retained the old frontend session');
      assert.deepEqual(resetReferers, [], 'Reset token URL was exposed in a Referer header');
      await page.getByLabel('Email Address', { exact: true }).fill(input.email);
      await page.getByLabel('Password', { exact: true }).fill(input.password);
      await page.getByRole('button', { name: 'Sign in with Email', exact: true }).click();
      await page.waitForURL('**/dashboard');
    } else {
      throw new Error('Unknown browser test mode');
    }
    const session = await page.evaluate(async () => {
      const response = await fetch('/api/auth/session');
      if (!response.ok) throw new Error('NextAuth session request failed');
      return response.json();
    });
    assert.equal(session.user.email, input.email);
    assert.ok(typeof session.jwt === 'string' && session.jwt.length > 20);
    assert.ok(session.roles.includes('admin'), 'First registered account must carry its assigned admin role');
    const usersResponse = await page.evaluate(async jwt => {
      const response = await fetch('/api/users', { headers: { Authorization: 'Bearer ' + jwt } });
      return { status: response.status, body: await response.json() };
    }, session.jwt);
    assert.equal(usersResponse.status, 200, 'Admin users request failed: ' + JSON.stringify(usersResponse.body));
    assert.ok(Array.isArray(usersResponse.body), 'Admin users response must be an array');
    assert.ok(usersResponse.body.some(user => user.email === input.email), 'Registered account missing from admin users API');
    await page.goto(input.url + '/admin/users');
    await page.getByRole('heading', { name: /users/i }).first().waitFor();
    await page.getByRole('row').filter({ hasText: input.email }).first().waitFor();
    assert.equal(new URL(page.url()).pathname, '/admin/users');
    assert.deepEqual(errors, [], 'Browser runtime errors');
    await page.screenshot({ path: input.screenshot, fullPage: true });
    // This test-only token is consumed in memory by the orchestrator, never logged.
    process.stdout.write(JSON.stringify({ jwt: session.jwt, userId: session.user.id }));
  } catch (error) {
    await page.screenshot({ path: input.screenshot, fullPage: true }).catch(() => {});
    throw error;
  } finally {
    await context.close();
    await browser.close();
  }
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
