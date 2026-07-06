import { expect, request, test, type Page } from '@playwright/test';

const BASE = process.env.BASE_URL ?? 'http://localhost:3000';
const EDGE_IP = `198.51.100.${Math.floor(Math.random() * 100) + 50}`;
const ADMIN_EMAIL = 'admin@massage.local';
const ADMIN_PASSWORD = 'admin1234';
const USER_EMAIL = 'user@massage.local';
const USER_PASSWORD = 'user1234';
let loginCounter = 0;


test.describe('운영 엣지케이스 안전성', () => {
  test.describe.configure({ timeout: 120_000 });

  test('malformed auth/session and missing shop media fail safely', async () => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const malformedLogin = await api.post('/api/auth/login', {
        data: '{bad',
        headers: {
          'content-type': 'application/json',
          'x-forwarded-for': EDGE_IP,
        },
      });
      expect(malformedLogin.status()).toBe(400);
      expect(malformedLogin.headers()['cache-control']).toContain('no-store');
      expect(malformedLogin.headers()['x-ratelimit-limit']).toBeTruthy();

      const tamperedSession = await api.get('/api/auth/me', {
        headers: { cookie: 'massage_session=bad.token.value' },
      });
      expect(tamperedSession.status()).toBe(200);
      expect(await tamperedSession.json()).toEqual({ user: null });

      const missingShop = await api.get('/api/shops/not-found-edge-slug-xyz');
      expect(missingShop.status()).toBe(404);

      const missingThumbnail = await api.get('/api/shops/not-found-edge-slug-xyz/thumbnail');
      expect(missingThumbnail.status()).toBe(404);
    } finally {
      await api.dispose();
    }
  });

  test('public shop directory clamps abusive pagination and tolerates invalid filters', async () => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const abusivePagination = await api.get('/api/shops?regularLimit=9999&regularOffset=-50');
      expect(abusivePagination.status()).toBe(200);
      const directoryBody = await abusivePagination.json() as { allShops?: unknown[]; regularShops?: unknown[] };
      expect(directoryBody.regularShops?.length ?? 0).toBeLessThanOrEqual(60);
      expect(directoryBody.allShops?.length ?? 0).toBeGreaterThanOrEqual(directoryBody.regularShops?.length ?? 0);

      const invalidTopRegion = await api.get('/api/shops/top?region=definitely-invalid');
      expect(invalidTopRegion.status()).toBe(200);
      const topBody = await invalidTopRegion.json();
      expect(Array.isArray(topBody) || Array.isArray(topBody.shops)).toBe(true);
    } finally {
      await api.dispose();
    }
  });

  test('mobile review create plus API update/delete works on a narrow viewport', async ({ page }) => {
    const reviewContent = `모바일 엣지 후기 ${Date.now()}`;
    const updatedContent = `${reviewContent} 수정`;
    let reviewId: string | null = null;

    await page.setViewportSize({ width: 390, height: 844 });
    await page.addInitScript(() => {
      localStorage.setItem('massage-adult-confirmed', 'true');
    });
    await addSessionToPage(page, USER_EMAIL, USER_PASSWORD);

    try {
      const api = await request.newContext({ baseURL: BASE });
      let shops: Array<{ slug: string }>;
      try {
        const topResponse = await api.get('/api/shops/top?region=seoul');
        expect(topResponse.status()).toBe(200);
        const topBody = await topResponse.json() as Array<{ slug: string }> | { shops: Array<{ slug: string }> };
        shops = Array.isArray(topBody) ? topBody : topBody.shops;
      } finally {
        await api.dispose();
      }
      expect(shops.length).toBeGreaterThan(0);

      await safeGoto(page, `${BASE}/shop/${shops[0].slug}`);
      const loginRequired = page.getByText('후기를 작성하려면 로그인이 필요합니다.');
      if (await loginRequired.isVisible().catch(() => false)) {
        await expect(loginRequired).not.toBeVisible({ timeout: 15_000 });
      }

      await page.getByText('방문 후기를 남겨주세요...').click();
      await page.locator('textarea').fill(reviewContent);
      const createResponsePromise = page.waitForResponse(
        (response) => response.url().includes('/api/board/reviews') && response.request().method() === 'POST',
      );
      await page.getByRole('button', { name: '후기 등록' }).click();
      const createResponse = await createResponsePromise;
      expect(createResponse.status()).toBe(201);
      const createBody = await createResponse.json() as { review?: { id?: string } };
      reviewId = createBody.review?.id ?? null;
      expect(reviewId).toBeTruthy();

      await safeGoto(page, `${BASE}/board/review`);
      await expect(page.getByText(reviewContent)).toBeVisible({ timeout: 10_000 });

      const updateOk = await page.evaluate(async ({ id, content }) => {
        const response = await fetch(`/api/board/reviews/${id}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ content, rating: 4 }),
        });
        return response.ok;
      }, { id: reviewId, content: updatedContent });
      expect(updateOk).toBe(true);

      await safeGoto(page, `${BASE}/board/review`);
      await expect(page.getByText(updatedContent)).toBeVisible({ timeout: 10_000 });

      const deleteOk = await page.evaluate(async (id) => {
        const response = await fetch(`/api/board/reviews/${id}`, { method: 'DELETE' });
        return response.ok;
      }, reviewId);
      expect(deleteOk).toBe(true);
      reviewId = null;
    } finally {
      if (reviewId) {
        await page.evaluate(async (id) => {
          await fetch(`/api/board/reviews/${id}`, { method: 'DELETE' });
        }, reviewId).catch(() => undefined);
      }
    }
  });

  test('admin upload rejects over-limit batches before storage writes', async () => {
    const admin = await newLoggedInContext(ADMIN_EMAIL, ADMIN_PASSWORD);
    try {
      const formData = new FormData();
      for (let index = 0; index < 11; index += 1) {
        formData.append('file', new File([PNG_1X1], `edge-${index}.png`, { type: 'image/png' }));
      }

      const response = await admin.post('/api/admin/upload', { multipart: formData });
      expect(response.status()).toBe(400);
      const body = await response.json() as { error?: string };
      expect(body.error).toContain('최대 10개');
    } finally {
      await admin.dispose();
    }
  });
});

async function newLoggedInContext(email: string, password: string) {
  loginCounter += 1;
  const context = await request.newContext({ baseURL: BASE });
  const response = await context.post('/api/auth/login', {
    data: { email, password },
    headers: { 'x-forwarded-for': `198.51.100.${100 + loginCounter}` },
  });
  expect(response.status()).toBe(200);
  return context;
}

async function addSessionToPage(page: Page, email: string, password: string) {
  const context = await newLoggedInContext(email, password);
  try {
    const state = await context.storageState();
    await page.context().addCookies(state.cookies);
  } finally {
    await context.dispose();
  }
}

async function safeGoto(page: Page, url: string) {
  try {
    await page.goto(url, { waitUntil: 'domcontentloaded' });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (!message.includes('interrupted') && !message.includes('NS_BINDING_ABORTED') && !message.includes('ERR_EMPTY_RESPONSE')) {
      throw error;
    }
    await page.waitForTimeout(500);
    await page.goto(url, { waitUntil: 'domcontentloaded' });
  }
}
const PNG_1X1 = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=',
  'base64',
);
