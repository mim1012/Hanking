import { expect, request, test } from '@playwright/test';

const BASE = process.env.BASE_URL ?? 'http://localhost:3000';
const RUN_ID = `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
const actors = {
  user: { email: 'user@massage.local', password: 'user1234' },
  owner: { email: 'owner@massage.local', password: 'owner1234' },
  admin: { email: 'admin@massage.local', password: 'admin1234' },
};

type Actor = keyof typeof actors;
type StorageState = Awaited<ReturnType<Awaited<ReturnType<typeof request.newContext>>['storageState']>>;

const storageStatePromises = new Map<Actor, Promise<StorageState>>();

function getStorageState(actor: Actor) {
  const existing = storageStatePromises.get(actor);
  if (existing) return existing;

  const promise = (async () => {
    const api = await request.newContext({ baseURL: BASE });
    try {
      const response = await api.post('/api/auth/login', { data: actors[actor] });
      expect(response.status()).toBe(200);
      return api.storageState();
    } finally {
      await api.dispose();
    }
  })();
  storageStatePromises.set(actor, promise);
  return promise;
}

async function contextFor(actor: keyof typeof actors | 'anonymous') {
  if (actor === 'anonymous') return request.newContext({ baseURL: BASE });

  return request.newContext({
    baseURL: BASE,
    storageState: await getStorageState(actor),
  });
}

const forbiddenAdminCases: Array<[keyof typeof actors | 'anonymous', string, 'get' | 'post' | 'patch' | 'delete', unknown?]> = [
  ['anonymous', '/api/admin/users', 'get'],
  ['user', '/api/admin/users', 'get'],
  ['owner', '/api/admin/users', 'get'],
  ['anonymous', '/api/admin/settings', 'patch', { siteName: 'blocked' }],
  ['user', '/api/admin/settings', 'patch', { siteName: 'blocked' }],
  ['owner', '/api/admin/settings', 'patch', { siteName: 'blocked' }],
  ['anonymous', '/api/admin/upload', 'post', {}],
  ['user', '/api/admin/upload', 'post', {}],
  ['owner', '/api/admin/themes', 'post', { code: `blocked_${RUN_ID}`, label: 'Blocked' }],
  ['user', '/api/admin/approvals', 'get'],
];

const malformedPublicCases: Array<[string, 'post' | 'patch' | 'delete', unknown, number[]]> = [
  ['/api/board/reviews', 'post', { rating: 5, content: 'missing shop' }, [401, 400]],
  ['/api/board/qna', 'post', { question: '' }, [400, 401]],
  ['/api/board/partnership', 'post', { name: '', phone: '', message: '' }, [400]],
  ['/api/auth/register/user', 'post', { email: 'not-email', password: 'short', name: '' }, [400]],
  ['/api/auth/register/owner', 'post', { email: 'not-email', password: 'short', name: '', businessName: '' }, [400]],
];

test.describe('운영 권한 negative matrix', () => {
  test.describe.configure({ timeout: 180_000 });

  for (const [actor, path, method, data] of forbiddenAdminCases) {
    test(`${actor} cannot ${method.toUpperCase()} ${path}`, async () => {
      const api = await contextFor(actor);
      try {
        const response = await api[method](path, data ? { data } : undefined);
        expect([401, 403, 405]).toContain(response.status());
        expect(response.headers()['cache-control'] ?? '').not.toContain('s-maxage');
      } finally {
        await api.dispose();
      }
    });
  }

  for (const [path, method, data, expectedStatuses] of malformedPublicCases) {
    test(`malformed public ${method.toUpperCase()} ${path} is rejected`, async () => {
      const api = await contextFor('anonymous');
      try {
        const response = await api[method](path, { data });
        expect(expectedStatuses).toContain(response.status());
      } finally {
        await api.dispose();
      }
    });
  }

  test('owner cannot create admin theme even with plausible payload', async () => {
    const api = await contextFor('owner');
    try {
      const response = await api.post('/api/admin/themes', { data: { code: `owner_blocked_${RUN_ID}`, label: 'Owner Blocked' } });
      expect([401, 403]).toContain(response.status());
    } finally {
      await api.dispose();
    }
  });

  test('user cannot delete arbitrary public review id', async () => {
    const api = await contextFor('user');
    try {
      const response = await api.delete('/api/board/reviews/not-a-real-review-id');
      expect([401, 403, 404]).toContain(response.status());
    } finally {
      await api.dispose();
    }
  });
});
