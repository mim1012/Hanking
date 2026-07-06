import { expect, request, test } from '@playwright/test';

const BASE = process.env.BASE_URL ?? 'http://localhost:3000';
const RUN_ID = `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;

const xssPayloads = [
  '<script>alert(1)</script>',
  '"><img src=x onerror=alert(1)>',
  '<svg onload=alert(1)>',
  'javascript:alert(1)',
];

const traversalPayloads = [
  'admin%2F..%2Fusers',
  '..%2Fadmin',
  '%2e%2e%2fapi%2fauth%2fme',
  'launch-scale-0001%00',
];

test.describe('운영 보안 fuzz smoke', () => {
  test.describe.configure({ timeout: 180_000 });

  for (const payload of xssPayloads) {
    test(`directory search treats XSS payload as inert text: ${payload.slice(0, 18)}`, async () => {
      const api = await request.newContext({ baseURL: BASE });
      try {
        const response = await api.get(`/api/shops?q=${encodeURIComponent(payload)}&regularLimit=5&prod-e2e=${RUN_ID}`);
        expect(response.status()).toBe(200);
        expect(response.headers()['content-type']).toContain('application/json');
        const raw = await response.text();
        expect(raw).not.toContain('<script>');
        expect(raw).not.toContain('onerror=');
        expect(raw).not.toContain('onload=');
      } finally {
        await api.dispose();
      }
    });
  }

  for (const payload of traversalPayloads) {
    test(`shop slug traversal payload does not escape route: ${payload}`, async () => {
      const api = await request.newContext({ baseURL: BASE });
      try {
        const response = await api.get(`/api/shops/${payload}`);
        expect([400, 404]).toContain(response.status());
        expect(response.url()).toContain('/api/shops/');
      } finally {
        await api.dispose();
      }
    });
  }

  test('admin upload rejects non-image masquerading as png', async () => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const login = await api.post('/api/auth/login', {
        data: { email: 'admin@massage.local', password: 'admin1234' },
        headers: { 'x-forwarded-for': `203.0.113.240-${RUN_ID}` },
      });
      if (login.status() === 429) test.skip(true, 'login rate limit active; covered by signed-session suites');
      expect(login.status()).toBe(200);

      const form = new FormData();
      form.append('file', new File(['not really png'], 'fake.png', { type: 'image/png' }));
      const response = await api.post('/api/admin/upload', { multipart: form });
      expect([400, 415]).toContain(response.status());
    } finally {
      await api.dispose();
    }
  });
});
