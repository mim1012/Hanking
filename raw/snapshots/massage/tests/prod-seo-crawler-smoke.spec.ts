import { expect, request, test } from '@playwright/test';

const BASE = process.env.BASE_URL ?? 'http://localhost:3000';

const htmlRoutes: Array<[string, RegExp]> = [
  ['/', /힐링찾기|마사지|디렉토리/],
  ['/top100', /TOP|인기|순위/],
  ['/board', /공지|Q&A|후기|커뮤니티/],
  ['/ad', /광고|입점|제휴/],
  ['/privacy', /개인정보/],
  ['/terms', /이용약관/],
  ['/youth', /청소년/],
  ['/shop/launch-scale-0001', /런칭 검증|업소|코스/],
];

const crawlerAgents = [
  'Googlebot/2.1 (+http://www.google.com/bot.html)',
  'Mozilla/5.0 AppleWebKit/537.36 Chrome/120 Safari/537.36',
];

test.describe('운영 SEO/crawler smoke', () => {
  test.describe.configure({ timeout: 180_000 });

  for (const agent of crawlerAgents) {
    for (const [path, expected] of htmlRoutes) {
      test(`crawler can load ${path} as ${agent.slice(0, 9)}`, async () => {
        const api = await request.newContext({ baseURL: BASE, userAgent: agent });
        try {
          const response = await api.get(path);
          expect(response.status()).toBe(200);
          expect(response.headers()['content-type']).toContain('text/html');
          const html = await response.text();
          expect(html).toMatch(expected);
          expect(html).toContain('<title>');
          expect(html).not.toContain('Application error');
        } finally {
          await api.dispose();
        }
      });
    }
  }

  test('rss route and robots-adjacent metadata stay reachable', async () => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const rss = await api.get('/rss');
      expect(rss.status()).toBe(200);
      expect(await rss.text()).toMatch(/rss|channel|힐링찾기/i);

      const home = await api.get('/');
      const html = await home.text();
      expect(html).toMatch(/canonical|og:title|description/i);
    } finally {
      await api.dispose();
    }
  });
});
