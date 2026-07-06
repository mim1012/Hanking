import { expect, request, test } from '@playwright/test';

const BASE = process.env.BASE_URL ?? 'http://localhost:3000';

const paths = [
  '/api/shops?regularLimit=60&regularOffset=0',
  '/api/shops?regularLimit=60&regularOffset=60',
  '/api/shops?region=seoul&sort=popular&regularLimit=60',
  '/api/shops?view=theme&theme=swedish&regularLimit=60',
  '/api/shops/top',
  '/api/shops/top?region=seoul',
  '/api/shops/launch-scale-0001',
  '/api/board/summary',
  '/api/ad-banners',
  '/api/themes',
];

type ProbeResult = {
  path: string;
  status: number;
  ms: number;
  cache: string;
};

async function timedGet(path: string): Promise<ProbeResult> {
  const api = await request.newContext({ baseURL: BASE });
  const startedAt = Date.now();
  try {
    const response = await api.get(path);
    await response.body();
    return {
      path,
      status: response.status(),
      ms: Date.now() - startedAt,
      cache: response.headers()['x-vercel-cache'] ?? 'n/a',
    };
  } finally {
    await api.dispose();
  }
}

test.describe('운영 latency/load smoke', () => {
  test.describe.configure({ timeout: 180_000 });

  test('warm public API endpoints stay below launch threshold under moderate concurrency', async () => {
    // Prime the CDN/cache once inside the test so this measures launch user experience after prewarm, not first cold MISS.
    await Promise.all(paths.map((path) => timedGet(path).catch(() => undefined)));

    const results = await Promise.all(
      Array.from({ length: 4 }).flatMap(() => paths.map((path) => timedGet(path))),
    );
    const failures = results.filter((result) => result.status >= 500);
    const slow = results.filter((result) => result.ms > 1500);

    expect(failures, JSON.stringify(failures, null, 2)).toHaveLength(0);
    expect(slow, JSON.stringify(slow, null, 2)).toHaveLength(0);

    const sorted = [...results].sort((left, right) => left.ms - right.ms);
    const p95 = sorted[Math.floor(sorted.length * 0.95)]?.ms ?? 0;
    expect(p95).toBeLessThanOrEqual(1200);
  });
});
