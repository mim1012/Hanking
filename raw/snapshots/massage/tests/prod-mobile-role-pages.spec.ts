import { expect, request, test, type Page } from '@playwright/test';
const BASE = process.env.BASE_URL ?? 'http://localhost:3000';
const credentials = {
  admin: { email: 'admin@massage.local', password: 'admin1234' },
  owner: { email: 'owner@massage.local', password: 'owner1234' },
  user: { email: 'user@massage.local', password: 'user1234' },
};

type Actor = keyof typeof credentials;

const storageStatePromises = new Map<Actor, Promise<Awaited<ReturnType<ReturnType<typeof request.newContext> extends Promise<infer Context> ? Context['storageState'] : never>>>>();

function getStorageState(actor: Actor) {
  const existing = storageStatePromises.get(actor);
  if (existing) return existing;

  const promise = (async () => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const response = await api.post('/api/auth/login', { data: credentials[actor] });
      expect(response.status()).toBe(200);
      return api.storageState();
    } finally {
      await api.dispose();
    }
  })();
  storageStatePromises.set(actor, promise);
  return promise;
}

async function addSession(page: Page, actor: keyof typeof credentials) {
  const state = await getStorageState(actor);
  await page.context().addCookies(state.cookies);
}

async function openMobile(page: Page, path: string) {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.addInitScript(() => localStorage.setItem('massage-adult-confirmed', 'true'));
  await page.goto(`${BASE}${path}`, { waitUntil: 'domcontentloaded', timeout: 60_000 });
  await expect(page.locator('header').first()).toBeVisible({ timeout: 30_000 });
  await expect(page.locator('main').last()).toBeVisible({ timeout: 30_000 });
}

const adminPages: Array<[string, RegExp]> = [
  ['/admin', /관리자|대시보드|통계/],
  ['/admin/shops', /업소 관리|업소/],
  ['/admin/approvals', /승인|입점|대기/],
  ['/admin/reviews', /리뷰|후기/],
  ['/admin/qna', /Q&A|문의/],
  ['/admin/users', /사용자|회원|권한|역할/],
  ['/admin/settings', /설정|사이트|SEO/],
  ['/admin/themes', /테마|스웨디시|아로마/],
  ['/admin/premium', /프리미엄|광고|추천/],
  ['/admin/stats', /통계|조회|방문/],
];

const ownerPages: Array<[string, RegExp]> = [
  ['/owner/shops', /내 업소|업소 관리|등록/],
  ['/owner/shops/new', /업소 등록|기본 정보|다음/],
  ['/owner/reviews', /리뷰|후기/],
  ['/owner/qna', /Q&A|문의/],
];

test.describe('운영 모바일 관리자/업주 페이지 접근성', () => {
  test.describe.configure({ timeout: 180_000 });

  for (const [path, expected] of adminPages) {
    test(`admin mobile page renders: ${path}`, async ({ page }) => {
      await addSession(page, 'admin');
      await openMobile(page, path);
      await expect(page.locator('main').last()).toContainText(expected, { timeout: 30_000 });
    });
  }

  for (const [path, expected] of ownerPages) {
    test(`owner mobile page renders: ${path}`, async ({ page }) => {
      await addSession(page, 'owner');
      await openMobile(page, path);
      await expect(page.locator('main').last()).toContainText(expected, { timeout: 30_000 });
    });
  }

  test('mobile anonymous protected pages redirect to login', async ({ page }) => {
    await openMobile(page, '/owner/shops');
    await expect.poll(() => page.url(), { timeout: 30_000 }).toContain('/auth/login');
  });

  test('mobile normal user cannot enter admin pages', async ({ page }) => {
    await addSession(page, 'user');
    await openMobile(page, '/admin/users');
    await expect.poll(() => page.url(), { timeout: 30_000 }).not.toContain('/admin/users');
  });
});
