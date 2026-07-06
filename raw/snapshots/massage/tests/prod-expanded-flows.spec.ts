import { expect, request, test, type APIRequestContext, type Page } from '@playwright/test';
import { prisma } from '@/lib/db/prisma';
import {
  DEFAULT_HOME_SEO,
  DEFAULT_SITE_SETTINGS,
  normalizeHomeSeo,
  normalizeSiteSettings,
} from '@/lib/site-content-defaults';
import { createSession } from '@/lib/server/auth-store';
const BASE = process.env.BASE_URL ?? 'http://localhost:3000';
const RUN_ID = `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
const ADMIN_EMAIL = 'admin@massage.local';
let adminStorageStatePromise: Promise<Awaited<ReturnType<APIRequestContext['storageState']>>> | null = null;
process.env.SESSION_SECRET ??= 'local-e2e-secret';

type JsonObject = Record<string, unknown>;

type SiteContentPayload = ReturnType<typeof normalizeSiteSettings> & ReturnType<typeof normalizeHomeSeo>;

async function getAdminStorageState() {
  adminStorageStatePromise ??= (async () => {
    const user = await prisma.user.findUniqueOrThrow({
      where: { email: ADMIN_EMAIL },
      select: { id: true, sessionVersion: true },
    });
    return {
      cookies: [
        {
          name: 'massage_session',
          value: createSession(user.id, user.sessionVersion),
          domain: new URL(BASE).hostname,
          path: '/',
          expires: Math.floor(Date.now() / 1000) + 60 * 60 * 24 * 7,
          httpOnly: true,
          secure: BASE.startsWith('https://'),
          sameSite: 'Lax' as const,
        },
      ],
      origins: [],
    };
  })();

  return adminStorageStatePromise;
}

async function newAdminContext() {
  return request.newContext({
    baseURL: BASE,
    storageState: await getAdminStorageState(),
  });
}

async function addAdminSessionToPage(page: Page) {
  const state = await getAdminStorageState();
  await page.context().addCookies(state.cookies);
}

async function withAdminContext<T>(fn: (context: APIRequestContext) => Promise<T>) {
  const context = await newAdminContext();
  try {
    return await fn(context);
  } finally {
    await context.dispose();
  }
}

async function json<T = JsonObject>(response: { json(): Promise<T> }) {
  return response.json();
}

function visibleMain(page: Page) {
  return page.locator('main').last();
}

function normalizeSiteContentPayload(source: JsonObject | null | undefined): SiteContentPayload {
  const nestedSettings = source?.siteSettings;
  const nestedSeo = source?.homeSeo;
  const flatSource = source ?? {};

  return {
    ...normalizeSiteSettings(
      typeof nestedSettings === 'object' && nestedSettings !== null
        ? nestedSettings as Partial<ReturnType<typeof normalizeSiteSettings>>
        : { ...DEFAULT_SITE_SETTINGS, ...flatSource },
    ),
    ...normalizeHomeSeo(
      typeof nestedSeo === 'object' && nestedSeo !== null
        ? nestedSeo as Partial<ReturnType<typeof normalizeHomeSeo>>
        : { ...DEFAULT_HOME_SEO, ...flatSource },
    ),
  };
}

test.describe('운영 확장 기능 플로우', () => {
  test.describe.configure({ timeout: 180_000 });
  test.setTimeout(180_000);

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('massage-adult-confirmed', 'true');
    });
  });

  test('public directory API and UI handle region/theme/sort/search/pagination together', async ({ page }) => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const listResponse = await api.get('/api/shops?region=seoul&theme=swedish&sort=popular&regularLimit=5');
      expect(listResponse.status()).toBe(200);
      const listBody = await json<{ allShops: Array<{ region: string; theme: string }>; regularTotal: number }>(listResponse);
      expect(listBody.allShops.length).toBeGreaterThan(0);
      expect(listBody.allShops.every((shop) => shop.region === 'seoul' && shop.theme === 'swedish')).toBe(true);
      expect(listBody.regularTotal).toBeGreaterThanOrEqual(0);

      await page.goto(`${BASE}/?view=theme&region=seoul&theme=swedish&sort=popular`, { waitUntil: 'domcontentloaded' });
      await expect(visibleMain(page)).toContainText(/서울|스웨디시/, { timeout: 30_000 });

      const searchResponse = await api.get('/api/shops?q=QA50&regularLimit=5');
      expect(searchResponse.status()).toBe(200);
      const searchBody = await json<{ allShops: Array<{ name: string; tags?: string[] }> }>(searchResponse);
      expect(searchBody.allShops.length).toBeGreaterThan(0);
      expect(searchBody.allShops.some((shop) => shop.name.includes('QA50') || shop.tags?.some((tag) => tag.includes('QA')))).toBe(true);

      await page.goto(`${BASE}/?q=QA50`, { waitUntil: 'domcontentloaded' });
      await expect(visibleMain(page)).toContainText(/QA50|검색/, { timeout: 30_000 });
    } finally {
      await api.dispose();
    }
  });

  test('top100, shop detail, reviews, and media endpoints load linked shop data', async ({ page }) => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const topResponse = await api.get('/api/shops/top?region=seoul');
      expect(topResponse.status()).toBe(200);
      const topBody = await json<Array<{ slug: string; name: string }> | { shops: Array<{ slug: string; name: string }> }>(topResponse);
      const topShops = Array.isArray(topBody) ? topBody : topBody.shops;
      const firstShop = topShops[0];
      expect(firstShop?.slug).toBeTruthy();

      const detailResponse = await api.get(`/api/shops/${encodeURIComponent(firstShop.slug)}`);
      expect(detailResponse.status()).toBe(200);
      const detailBody = await json<{ shop: { slug: string; name: string }; reviews: unknown[] }>(detailResponse);
      expect(detailBody.shop.slug).toBe(firstShop.slug);
      expect(Array.isArray(detailBody.reviews)).toBe(true);

      const reviewsResponse = await api.get(`/api/shops/${encodeURIComponent(firstShop.slug)}/reviews`);
      expect([200, 401]).toContain(reviewsResponse.status());
      const reviewsBody = await json<{ reviews?: unknown[] }>(reviewsResponse);
      expect(Array.isArray(reviewsBody.reviews)).toBe(true);

      const thumbnailResponse = await api.get(`/api/shops/${encodeURIComponent(firstShop.slug)}/thumbnail?size=card`);
      expect([200, 204, 302, 404]).toContain(thumbnailResponse.status());
      const bannerResponse = await api.get(`/api/shops/${encodeURIComponent(firstShop.slug)}/banner?size=detail`);
      expect([200, 204, 302, 404]).toContain(bannerResponse.status());
      const galleryResponse = await api.get(`/api/shops/${encodeURIComponent(firstShop.slug)}/images/0?size=detail`);
      expect([200, 204, 302, 404]).toContain(galleryResponse.status());

      await page.goto(`${BASE}/top100?region=seoul`, { waitUntil: 'domcontentloaded' });
      await expect(visibleMain(page)).toContainText(/TOP|인기|순위/, { timeout: 30_000 });
      await page.goto(`${BASE}/shop/${firstShop.slug}`, { waitUntil: 'domcontentloaded' });
      await expect(visibleMain(page)).toContainText(firstShop.name, { timeout: 30_000 });
    } finally {
      await api.dispose();
    }
  });

  test('admin settings can be saved, reloaded, and restored on production', async () => {
    await withAdminContext(async (admin) => {
      const originalResponse = await admin.get('/api/admin/settings');
      expect(originalResponse.status()).toBe(200);
      const originalAdmin = await json<JsonObject | null>(originalResponse);
      const original = normalizeSiteContentPayload(
        originalAdmin ?? await admin.get(`/api/site-settings?prod-e2e=${RUN_ID}`).then(async (response) => {
          expect(response.status()).toBe(200);
          return json<JsonObject | null>(response);
        }),
      );
      const marker = `운영설정검증-${RUN_ID}`;
      const patched: SiteContentPayload = {
        ...original,
        heroSubText: `${original.heroSubText} ${marker}`,
      };

      const patchResponse = await admin.patch('/api/admin/settings', { data: patched });
      expect(patchResponse.status()).toBe(200);
      try {
        const reloadResponse = await admin.get(`/api/admin/settings?prod-e2e=${RUN_ID}`);
        expect(reloadResponse.status()).toBe(200);
        const reloaded = normalizeSiteContentPayload(await json<JsonObject | null>(reloadResponse));
        expect(reloaded.heroSubText).toContain(marker);
      } finally {
        const restoreResponse = await admin.patch('/api/admin/settings', { data: original });
        expect(restoreResponse.status()).toBe(200);
      }
    });
  });

  test('admin theme CRUD propagates to the public themes API and cleans up', async () => {
    const code = `prod_e2e_${RUN_ID.replace(/[^a-zA-Z0-9]/g, '_')}`.toLowerCase();
    await prisma.shop.deleteMany({ where: { theme: code } });
    await prisma.theme.deleteMany({ where: { code } });

    await withAdminContext(async (admin) => {
      try {
        const createResponse = await admin.post('/api/admin/themes', {
          data: { code, label: `운영 E2E 테마 ${RUN_ID}`, emoji: '✅' },
        });
        expect(createResponse.status()).toBe(201);

        const publicResponse = await admin.get(`/api/themes?prod-e2e=${RUN_ID}`);
        expect(publicResponse.status()).toBe(200);
        const publicBody = await json<{ themes: Array<{ code: string; label: string }> }>(publicResponse);
        expect(publicBody.themes.some((theme) => theme.code === code)).toBe(true);
      } finally {
        await admin.delete('/api/admin/themes', { data: { code } }).catch(() => undefined);
        await prisma.shop.deleteMany({ where: { theme: code } });
        await prisma.theme.deleteMany({ where: { code } });
      }

      const afterDeleteResponse = await admin.get(`/api/themes?prod-e2e=${RUN_ID}-deleted`);
      expect(afterDeleteResponse.status()).toBe(200);
      const afterDeleteBody = await json<{ themes: Array<{ code: string }> }>(afterDeleteResponse);
      expect(afterDeleteBody.themes.some((theme) => theme.code === code)).toBe(false);
    });
  });

  test('admin ad banner edit updates public banner payload and restores original slot', async () => {
    await withAdminContext(async (admin) => {
      const slot = 'mobile';
      const allResponse = await admin.get('/api/admin/ad-banners');
      expect(allResponse.status()).toBe(200);
      const allBody = await json<{ banners: Array<{ slot: string; imageUrl: string; linkUrl: string | null; isActive: boolean }> }>(allResponse);
      const original = allBody.banners.find((banner) => banner.slot === slot) ?? { slot, imageUrl: '', linkUrl: null, isActive: false };

      const testBanner = {
        slot,
        imageUrl: '/favicon.ico',
        linkUrl: `/board/notice?prod-e2e=${RUN_ID}`,
        isActive: true,
      };
      const patchResponse = await admin.patch('/api/admin/ad-banners', { data: testBanner });
      expect(patchResponse.status()).toBe(200);
      try {
        const publicResponse = await admin.get(`/api/ad-banners?prod-e2e=${RUN_ID}`);
        expect(publicResponse.status()).toBe(200);
        const publicBody = await json<{ banners: Record<string, { imageUrl: string; linkUrl: string | null }> }>(publicResponse);
        expect(publicBody.banners[slot]?.imageUrl).toBe(testBanner.imageUrl);
        expect(publicBody.banners[slot]?.linkUrl).toBe(testBanner.linkUrl);
      } finally {
        const restoreResponse = await admin.patch('/api/admin/ad-banners', { data: original });
        expect(restoreResponse.status()).toBe(200);
      }
    });
  });

  test('admin users, stats, and dashboard data load without client-side loading dead ends', async ({ page }) => {
    await addAdminSessionToPage(page);
    await withAdminContext(async (admin) => {
      const usersResponse = await admin.get('/api/admin/users?page=1&pageSize=5&role=OWNER');
      expect(usersResponse.status()).toBe(200);
      const usersBody = await json<{ users: unknown[]; total: number }>(usersResponse);
      expect(Array.isArray(usersBody.users)).toBe(true);
      expect(usersBody.total).toBeGreaterThanOrEqual(usersBody.users.length);

      const statsResponse = await admin.get('/api/admin/stats');
      expect(statsResponse.status()).toBe(200);
      const dashboardResponse = await admin.get('/api/admin/dashboard');
      expect(dashboardResponse.status()).toBe(200);
    });

    await page.goto(`${BASE}/admin/users`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('main').last()).toContainText(/사용자|회원|권한|역할/, { timeout: 30_000 });
    await page.goto(`${BASE}/admin/stats`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('main').last()).toContainText(/통계|조회|방문|페이지/, { timeout: 30_000 });
  });

  test('analytics page-view tracking deduplicates repeated events for the same production session', async () => {
    const path = `/shop/healing-spa-seoul?prod-e2e=${RUN_ID}`;
    await prisma.pageViewEvent.deleteMany({ where: { path } });
    const api = await request.newContext({ baseURL: BASE });
    try {
      const firstResponse = await api.post('/api/analytics/page-view', {
        data: { path, referrer: `${BASE}/?prod-e2e=${RUN_ID}` },
      });
      expect(firstResponse.status()).toBe(200);
      const secondResponse = await api.post('/api/analytics/page-view', {
        data: { path, referrer: `${BASE}/?prod-e2e=${RUN_ID}` },
      });
      expect(secondResponse.status()).toBe(200);

      await expect
        .poll(async () => prisma.pageViewEvent.count({ where: { path } }), { timeout: 10_000 })
        .toBe(1);
    } finally {
      await api.dispose();
      await prisma.pageViewEvent.deleteMany({ where: { path } });
    }
  });
});
