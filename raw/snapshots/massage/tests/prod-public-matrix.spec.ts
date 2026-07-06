import { expect, request, test } from '@playwright/test';

const BASE = process.env.BASE_URL ?? 'http://localhost:3000';

type DirectoryBody = {
  allShops?: Array<{ slug?: string; region?: string; theme?: string; isPremium?: boolean }>;
  premiumShops?: unknown[];
  regularShops?: unknown[];
  regularTotal?: number;
  total?: number;
};

const regionCases = ['seoul', 'gyeonggi', 'busan', 'daegu', 'jeju', 'gwangju'];
const themeCases = ['swedish', 'aroma', 'thai', 'sport', 'deep', 'foot'];
const sortCases = ['popular', 'new'];

async function getWithRetry(api: Awaited<ReturnType<typeof request.newContext>>, path: string) {
  let lastError: unknown;
  for (let attempt = 0; attempt < 3; attempt += 1) {
    try {
      const response = await api.get(path);
      if (response.status() < 500 || attempt === 2) {
        return response;
      }
    } catch (error) {
      lastError = error;
      if (attempt === 2) throw error;
    }
    await new Promise((resolve) => setTimeout(resolve, 750));
  }
  throw lastError;
}
test.describe('운영 public 디렉토리 필터 행렬', () => {
  test.describe.configure({ timeout: 180_000 });

  for (const region of regionCases) {
    test(`region filter returns bounded cacheable payload: ${region}`, async () => {
      const api = await request.newContext({ baseURL: BASE });
      try {
        const response = await getWithRetry(api, `/api/shops?region=${region}&regularLimit=12`);
        expect(response.status()).toBe(200);
        expect(response.headers()['cache-control']).toContain('public');
        const body = await response.json() as DirectoryBody;
        expect(body.regularShops?.length ?? 0).toBeLessThanOrEqual(12);
        expect(body.allShops ?? []).toEqual(expect.any(Array));
        expect((body.allShops ?? []).every((shop) => !shop.region || shop.region === region)).toBe(true);
      } finally {
        await api.dispose();
      }
    });
  }

  for (const theme of themeCases) {
    test(`theme filter returns bounded cacheable payload: ${theme}`, async () => {
      const api = await request.newContext({ baseURL: BASE });
      try {
        const response = await getWithRetry(api, `/api/shops?view=theme&theme=${theme}&regularLimit=12`);
        expect(response.status()).toBe(200);
        expect(response.headers()['cache-control']).toContain('public');
        const body = await response.json() as DirectoryBody;
        expect(body.regularShops?.length ?? 0).toBeLessThanOrEqual(12);
        expect((body.allShops ?? []).every((shop) => !shop.theme || shop.theme === theme)).toBe(true);
      } finally {
        await api.dispose();
      }
    });
  }

  for (const sort of sortCases) {
    test(`sort mode stays cacheable and paginated: ${sort}`, async () => {
      const api = await request.newContext({ baseURL: BASE });
      try {
        const response = await getWithRetry(api, `/api/shops?sort=${sort}&regularLimit=15&regularOffset=15`);
        expect(response.status()).toBe(200);
        expect(response.headers()['cache-control']).toContain('public');
        const body = await response.json() as DirectoryBody;
        expect(body.premiumShops?.length ?? 0).toBe(0);
        expect(body.regularShops?.length ?? 0).toBeLessThanOrEqual(15);
        expect(body.regularTotal ?? 0).toBeGreaterThanOrEqual(body.regularShops?.length ?? 0);
      } finally {
        await api.dispose();
      }
    });
  }

  for (const region of ['seoul', 'gyeonggi', 'busan', 'jeju']) {
    for (const theme of ['swedish', 'aroma', 'thai']) {
      test(`region+theme+popular matrix is safe: ${region}/${theme}`, async () => {
        const api = await request.newContext({ baseURL: BASE });
        try {
          const response = await getWithRetry(api, `/api/shops?region=${region}&theme=${theme}&sort=popular&regularLimit=8`);
          expect(response.status()).toBe(200);
          const body = await response.json() as DirectoryBody;
          expect(body.regularShops?.length ?? 0).toBeLessThanOrEqual(8);
          expect((body.allShops ?? []).every((shop) => (!shop.region || shop.region === region) && (!shop.theme || shop.theme === theme))).toBe(true);
        } finally {
          await api.dispose();
        }
      });
    }
  }

  for (const region of ['seoul', 'gyeonggi', 'busan', 'jeju']) {
    test(`top100 regional payload is cacheable: ${region}`, async () => {
      const api = await request.newContext({ baseURL: BASE });
      try {
        const response = await getWithRetry(api, `/api/shops/top?region=${region}`);
        expect(response.status()).toBe(200);
        expect(response.headers()['cache-control']).toContain('public');
        const body = await response.json() as Array<{ region?: string }> | { shops?: Array<{ region?: string }> };
        const shops = Array.isArray(body) ? body : body.shops ?? [];
        expect(shops.length).toBeLessThanOrEqual(100);
        expect(shops.every((shop) => !shop.region || shop.region === region)).toBe(true);
      } finally {
        await api.dispose();
      }
    });
  }
});
