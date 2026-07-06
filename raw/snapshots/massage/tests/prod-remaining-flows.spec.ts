import { expect, request, test, type APIRequestContext, type Page } from '@playwright/test';
import { UserRole, UserStatus } from '@prisma/client';
import { prisma } from '@/lib/db/prisma';
import { hashPassword } from '@/lib/auth/password';
import { createSession } from '@/lib/server/auth-store';
import type { Shop } from '@/lib/types';

const BASE = process.env.BASE_URL ?? 'http://localhost:3000';
const RUN_ID = `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
const ADMIN_EMAIL = 'admin@massage.local';
let adminStorageStatePromise: Promise<Awaited<ReturnType<APIRequestContext['storageState']>>> | null = null;
process.env.SESSION_SECRET ??= 'local-e2e-secret';

type JsonObject = Record<string, unknown>;

test.describe('운영 잔여 기능 플로우', () => {
  test.describe.configure({ timeout: 180_000 });
  test.setTimeout(180_000);

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('massage-adult-confirmed', 'true');
    });
  });

  test('admin shop CRUD, visibility, premium, and public directory reflection', async ({ page }) => {
    const marker = `운영 잔여 업소 ${RUN_ID}`;
    const updatedMarker = `${marker} 수정`;
    const createdIds: string[] = [];

    await withAdminContext(async (admin) => {
      try {
        const createResponse = await admin.post('/api/admin/shops', {
          data: {
            shop: buildTestShop(marker),
          },
        });
        expect(createResponse.status()).toBe(201);
        const createBody = await json<{ shop: Shop }>(createResponse);
        createdIds.push(createBody.shop.id);
        expect(createBody.shop.name).toBe(marker);
        expect(createBody.shop.slug).toBeTruthy();

        const publicCreateResponse = await admin.get(`/api/shops?q=${encodeURIComponent(marker)}&regularLimit=5&prod-e2e=${RUN_ID}`);
        expect(publicCreateResponse.status()).toBe(200);
        const publicCreateBody = await json<{ allShops: Array<{ id: string; name: string }> }>(publicCreateResponse);
        expect(publicCreateBody.allShops.some((shop) => shop.id === createBody.shop.id)).toBe(true);

        const patchResponse = await admin.patch(`/api/admin/shops/${createBody.shop.id}`, {
          data: {
            shop: {
              ...createBody.shop,
              name: updatedMarker,
              tagline: '운영 잔여 업소 수정 태그라인',
              description: '운영 잔여 업소 수정 설명',
            },
          },
        });
        expect(patchResponse.status()).toBe(200);
        const patchBody = await json<{ shop: Shop }>(patchResponse);
        expect(patchBody.shop.name).toBe(updatedMarker);

        const hideResponse = await admin.patch(`/api/admin/shops/${createBody.shop.id}/visibility`, { data: { isVisible: false } });
        expect(hideResponse.status()).toBe(200);
        const hiddenSearch = await admin.get(`/api/shops?q=${encodeURIComponent(updatedMarker)}&regularLimit=5&prod-e2e=${RUN_ID}-hidden`);
        expect(hiddenSearch.status()).toBe(200);
        const hiddenSearchBody = await json<{ allShops: Array<{ id: string }> }>(hiddenSearch);
        expect(hiddenSearchBody.allShops.some((shop) => shop.id === createBody.shop.id)).toBe(false);

        const showResponse = await admin.patch(`/api/admin/shops/${createBody.shop.id}/visibility`, { data: { isVisible: true } });
        expect(showResponse.status()).toBe(200);
        const premiumResponse = await admin.patch(`/api/admin/shops/${createBody.shop.id}/premium`, {
          data: { isPremium: true, premiumOrder: 1 },
        });
        expect(premiumResponse.status()).toBe(200);
        const premiumBody = await json<{ shop: Shop }>(premiumResponse);
        expect(premiumBody.shop.isPremium).toBe(true);

        await addAdminSessionToPage(page);
        await page.goto(`${BASE}/admin/shops`, { waitUntil: 'domcontentloaded' });
        await expect(page.locator('main').last()).toContainText(/업소 관리|업소/, { timeout: 30_000 });
      } finally {
        await cleanupShops(createdIds);
      }
    });
  });

  test('admin upload accepts real image files and rejects disguised files', async () => {
    await withAdminContext(async (admin) => {
      const badUpload = await admin.post('/api/admin/upload', {
        multipart: {
          file: {
            name: `not-image-${RUN_ID}.png`,
            mimeType: 'image/png',
            buffer: Buffer.from('not a png'),
          },
        },
      });
      expect(badUpload.status()).toBe(400);
      const badBody = await json<{ error: string }>(badUpload);
      expect(badBody.error).toContain('실제 이미지 파일');

      const goodUpload = await admin.post('/api/admin/upload', {
        multipart: {
          file: {
            name: `prod-e2e-${RUN_ID}.png`,
            mimeType: 'image/png',
            buffer: PNG_1X1,
          },
        },
      });
      expect(goodUpload.status()).toBe(201);
      const goodBody = await json<{ urls: string[] }>(goodUpload);
      expect(goodBody.urls).toHaveLength(1);
      expect(goodBody.urls[0]).toMatch(/\.png($|\?)/);
      await deleteUploadedObject(goodBody.urls[0]);
    });
  });

  test('admin user management updates editable fields and blocks invalid status changes', async () => {
    const userId = `prod-user-${RUN_ID}`;
    const email = `${userId}@massage.local`;
    await prisma.user.deleteMany({ where: { id: userId } });

    try {
      await prisma.user.create({
        data: {
          id: userId,
          email,
          passwordHash: hashPassword('Prod-user-1234'),
          name: '운영 회원관리 대상',
          role: UserRole.USER,
          status: UserStatus.APPROVED,
        },
      });

      await withAdminContext(async (admin) => {
        const listResponse = await admin.get(`/api/admin/users?q=${encodeURIComponent(email)}&page=1&pageSize=5`);
        expect(listResponse.status()).toBe(200);
        const listBody = await json<{ users: Array<{ id: string; email: string }> }>(listResponse);
        expect(listBody.users.some((user) => user.id === userId)).toBe(true);

        const updateResponse = await admin.patch('/api/admin/users', {
          data: { id: userId, name: '운영 회원관리 수정', phone: '010-7777-8888' },
        });
        expect(updateResponse.status()).toBe(200);
        const updateBody = await json<{ user: { name: string; phone?: string } }>(updateResponse);
        expect(updateBody.user.name).toBe('운영 회원관리 수정');
        expect(updateBody.user.phone).toBe('010-7777-8888');

        const invalidStatusResponse = await admin.patch('/api/admin/users', {
          data: { id: userId, status: 'rejected' },
        });
        expect(invalidStatusResponse.status()).toBe(400);
        const invalidStatusBody = await json<{ error: string }>(invalidStatusResponse);
        expect(invalidStatusBody.error).toContain('업체관리자 승인 상태');
      });
    } finally {
      await prisma.user.deleteMany({ where: { id: userId } });
    }
  });

  test('legal document editing is reflected on public pages and restored', async ({ page }) => {
    await withAdminContext(async (admin) => {
      const originalResponse = await admin.get('/api/admin/legal-documents');
      expect(originalResponse.status()).toBe(200);
      const originalDocuments = await json<Record<string, JsonObject>>(originalResponse);
      const originalPrivacy = originalDocuments.privacy;
      expect(typeof originalPrivacy.title).toBe('string');

      const marker = `운영 개인정보 검증 ${RUN_ID}`;
      const patched = {
        slug: 'privacy',
        eyebrow: String(originalPrivacy.eyebrow),
        title: `${String(originalPrivacy.title)} ${marker}`,
        description: String(originalPrivacy.description),
        note: String(originalPrivacy.note ?? ''),
        body: `${String(originalPrivacy.body)}\n\n${marker}`,
      };

      const patchResponse = await admin.patch('/api/admin/legal-documents', { data: patched });
      expect(patchResponse.status()).toBe(200);
      try {
        await page.goto(`${BASE}/privacy?prod-e2e=${RUN_ID}`, { waitUntil: 'domcontentloaded' });
        await expect(page.locator('main').last()).toContainText(marker, { timeout: 30_000 });
      } finally {
        const restoreResponse = await admin.patch('/api/admin/legal-documents', {
          data: {
            slug: 'privacy',
            eyebrow: originalPrivacy.eyebrow,
            title: originalPrivacy.title,
            description: originalPrivacy.description,
            note: originalPrivacy.note,
            body: originalPrivacy.body,
          },
        });
        expect(restoreResponse.status()).toBe(200);
      }
    });
  });

  test('partnership inquiry public create, admin status update, and delete flow', async () => {
    const api = await request.newContext({ baseURL: BASE });
    let inquiryId: string | null = null;
    try {
      const createResponse = await api.post('/api/board/partnership', {
        data: {
          shopName: `운영 제휴 ${RUN_ID}`,
          region: 'seoul',
          subRegion: 'gangnam',
          theme: 'swedish',
          contactName: '운영 제휴 담당자',
          phone: '010-1212-3434',
          kakaoId: `prod-${RUN_ID}`,
          message: `운영 제휴 문의 ${RUN_ID}`,
        },
      });
      expect(createResponse.status()).toBe(201);
      const createBody = await json<{ inquiry: { id: string; status: string } }>(createResponse);
      inquiryId = createBody.inquiry.id;
      expect(createBody.inquiry.status).toBe('pending');

      await withAdminContext(async (admin) => {
        const listResponse = await admin.get('/api/admin/partnerships');
        expect(listResponse.status()).toBe(200);
        const listBody = await json<{ inquiries: Array<{ id: string }> }>(listResponse);
        expect(listBody.inquiries.some((inquiry) => inquiry.id === inquiryId)).toBe(true);

        const statusResponse = await admin.patch(`/api/admin/partnerships/${inquiryId}`, { data: { status: 'contacted' } });
        expect(statusResponse.status()).toBe(200);
        const statusBody = await json<{ inquiry: { status: string } }>(statusResponse);
        expect(statusBody.inquiry.status).toBe('contacted');

        const deleteResponse = await admin.delete(`/api/admin/partnerships/${inquiryId}`);
        expect(deleteResponse.status()).toBe(204);
        inquiryId = null;
      });
    } finally {
      await api.dispose();
      if (inquiryId) {
        await prisma.partnershipInquiry.deleteMany({ where: { id: inquiryId } });
      }
    }
  });
  test('low-coverage public aliases, owner Q&A, and partnership settings stay reachable', async ({ page }) => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const partnershipSettings = await api.get('/api/board/partnership/settings');
      expect(partnershipSettings.status()).toBe(200);
      const settingsBody = await json<{ title?: string; body?: string }>(partnershipSettings);
      expect(String(settingsBody.title ?? settingsBody.body ?? '')).not.toBe('');

      await page.goto(`${BASE}/ads`, { waitUntil: 'domcontentloaded' });
      await expect(page).toHaveURL(/\/ad(?:[?#].*)?$/);
      await expect(page.locator('main').last()).toContainText(/광고|제휴|입점/, { timeout: 30_000 });

      await page.goto(`${BASE}/advertise`, { waitUntil: 'domcontentloaded' });
      await expect(page).toHaveURL(/\/ad(?:[?#].*)?$/);

      await page.goto(`${BASE}/auth/register-owner/success`, { waitUntil: 'domcontentloaded' });
      await expect(page.locator('main').last()).toContainText(/입점사|신청|승인/, { timeout: 30_000 });

      await addAdminSessionToPage(page);
      await page.goto(`${BASE}/owner/qna`, { waitUntil: 'domcontentloaded' });
      await expect(page.locator('main').last()).toContainText(/내 업소 Q&A 댓글 관리|Q&A 댓글 관리/, { timeout: 30_000 });
    } finally {
      await api.dispose();
    }
  });

  test('mobile viewport covers public navigation, search, detail, and auth UI', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`${BASE}/?q=QA50`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('header')).toBeVisible({ timeout: 30_000 });
    await expect(page.locator('main').last()).toContainText(/QA50|검색/, { timeout: 30_000 });

    const api = await request.newContext({ baseURL: BASE });
    try {
      const topResponse = await api.get('/api/shops/top?region=seoul');
      expect(topResponse.status()).toBe(200);
      const topBody = await json<Array<{ slug: string; name: string }> | { shops: Array<{ slug: string; name: string }> }>(topResponse);
      const shops = Array.isArray(topBody) ? topBody : topBody.shops;
      expect(shops.length).toBeGreaterThan(0);

      await page.goto(`${BASE}/shop/${shops[0].slug}`, { waitUntil: 'domcontentloaded' });
      await expect(page.locator('main').last()).toContainText(shops[0].name, { timeout: 30_000 });
      await page.goto(`${BASE}/auth/login`, { waitUntil: 'domcontentloaded' });
      await expect(page.getByRole('heading', { name: '일반 회원 로그인' })).toBeVisible({ timeout: 30_000 });
      await page.getByRole('button', { name: /입점사\(업체\)/ }).click();
      await expect(page.getByRole('heading', { name: '사장님 로그인' })).toBeVisible({ timeout: 30_000 });
    } finally {
      await api.dispose();
    }
  });

  test('negative API paths return explicit auth and validation errors', async () => {
    const anonymous = await request.newContext({ baseURL: BASE });
    try {
      const adminUsers = await anonymous.get('/api/admin/users');
      expect([401, 403]).toContain(adminUsers.status());

      const badPartnership = await anonymous.post('/api/board/partnership', { data: { shopName: '' } });
      expect(badPartnership.status()).toBe(400);

      const badShopQuery = await anonymous.get('/api/shops?region=not-a-region&regularLimit=5');
      expect([200, 400, 404]).toContain(badShopQuery.status());
    } finally {
      await anonymous.dispose();
    }

    await withAdminContext(async (admin) => {
      const badLegal = await admin.patch('/api/admin/legal-documents', { data: { slug: 'unknown' } });
      expect(badLegal.status()).toBe(400);

      const badPremium = await admin.patch('/api/admin/shops/not-found/premium', { data: { isPremium: 'yes' } });
      expect(badPremium.status()).toBe(400);
    });
  });
});

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

async function withAdminContext<T>(fn: (context: APIRequestContext) => Promise<T>) {
  const context = await newAdminContext();
  try {
    return await fn(context);
  } finally {
    await context.dispose();
  }
}

async function addAdminSessionToPage(page: Page) {
  const state = await getAdminStorageState();
  await page.context().addCookies(state.cookies);
}

async function json<T = JsonObject>(response: { json(): Promise<T> }) {
  return response.json();
}

function buildTestShop(name: string): Shop {
  return {
    id: '',
    name,
    slug: '',
    region: 'seoul',
    regionLabel: '서울',
    subRegion: 'gangnam',
    subRegionLabel: '강남',
    theme: 'swedish',
    themeLabel: '스웨디시',
    isPremium: false,
    thumbnailUrl: '/favicon.ico',
    bannerUrl: '/favicon.ico',
    detailImageUrl: '/favicon.ico',
    tagline: '운영 잔여 업소 태그라인',
    rating: 0,
    reviewCount: 0,
    courses: [{ name: '기본 코스', duration: '60', price: '100000', description: '운영 잔여 코스' }],
    tags: ['운영검증', '잔여시나리오'],
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    images: ['/favicon.ico'],
    description: '운영 잔여 업소 설명',
    address: '서울 강남구 운영로 1',
    phone: '02-1234-5678',
    hours: '10:00-22:00',
    isVisible: true,
  };
}

async function cleanupShops(ids: string[]) {
  if (ids.length === 0) return;
  await prisma.review.deleteMany({ where: { shopId: { in: ids } } });
  await prisma.qnA.deleteMany({ where: { shopId: { in: ids } } });
  await prisma.shop.deleteMany({ where: { id: { in: ids } } });
}

async function deleteUploadedObject(publicUrl: string | undefined) {
  if (!publicUrl || !process.env.SUPABASE_URL || !process.env.SUPABASE_SERVICE_ROLE_KEY) return;
  const url = new URL(publicUrl);
  const marker = '/storage/v1/object/public/shop-images/';
  const markerIndex = url.pathname.indexOf(marker);
  if (markerIndex === -1) return;
  const objectPath = decodeURIComponent(url.pathname.slice(markerIndex + marker.length));
  const origin = new URL(process.env.SUPABASE_URL).origin;
  await fetch(`${origin}/storage/v1/object/shop-images/${encodeURIComponent(objectPath).replace(/%2F/g, '/')}`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${process.env.SUPABASE_SERVICE_ROLE_KEY}`,
      apikey: process.env.SUPABASE_SERVICE_ROLE_KEY,
    },
  }).catch(() => undefined);
}

const PNG_1X1 = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=',
  'base64',
);
