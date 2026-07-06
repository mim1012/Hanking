type MemoryRateLimiterOptions = {
  limit?: number;
  windowMs?: number;
  now?: () => number;
};

type RateLimitBucket = {
  count: number;
  resetAt: number;
};

export type RateLimitCheckResult =
  | {
      limited: false;
      headers: Headers;
    }
  | {
      limited: true;
      response: Response;
    };

export type AuthRateLimitChecker = (
  request: Pick<Request, 'headers'>,
  routeKey: string,
  options?: { credential?: string },
) => RateLimitCheckResult | Promise<RateLimitCheckResult>;

const DEFAULT_LIMIT = 10;
const DEFAULT_WINDOW_MS = 60_000;
const AUTH_LOGIN_IP_LIMIT = 30;
const AUTH_LOGIN_CREDENTIAL_LIMIT = 5;
const AUTH_LOGIN_WINDOW_MS = 10 * 60_000;
const TOO_MANY_REQUESTS_MESSAGE = '요청이 너무 많습니다. 잠시 후 다시 시도해 주세요.';

type AuthRateLimitOptions = {
  credential?: string;
};

export function getClientIp(request: Pick<Request, 'headers'>) {
  // 플랫폼(Vercel/프록시)이 설정하는 헤더를 우선한다 — x-forwarded-for 좌측값은 클라이언트가 위조할 수 있다.
  const platformIp =
    request.headers.get('x-real-ip') ??
    request.headers.get('x-vercel-forwarded-for') ??
    request.headers.get('cf-connecting-ip');
  if (platformIp?.trim()) {
    return platformIp.trim();
  }

  const forwardedFor = request.headers.get('x-forwarded-for');
  if (forwardedFor) {
    const [firstIp] = forwardedFor.split(',');
    if (firstIp?.trim()) {
      return firstIp.trim();
    }
  }

  return 'unknown';
}

export function createMemoryRateLimiter(options: MemoryRateLimiterOptions = {}) {
  const limit = options.limit ?? DEFAULT_LIMIT;
  const windowMs = options.windowMs ?? DEFAULT_WINDOW_MS;
  const now = options.now ?? (() => Date.now());
  const buckets = new Map<string, RateLimitBucket>();
  let nextSweepAt = 0;

  function sweepExpiredBuckets(currentTime: number) {
    if (currentTime < nextSweepAt) {
      return;
    }

    nextSweepAt = currentTime + windowMs;
    for (const [key, bucket] of buckets) {
      if (bucket.resetAt <= currentTime) {
        buckets.delete(key);
      }
    }
  }

  function buildHeaders(bucket: RateLimitBucket) {
    const remaining = Math.max(0, limit - bucket.count);
    const headers = new Headers();
    headers.set('Cache-Control', 'no-store');
    headers.set('Retry-After', String(Math.max(1, Math.ceil((bucket.resetAt - now()) / 1000))));
    headers.set('X-RateLimit-Limit', String(limit));
    headers.set('X-RateLimit-Remaining', String(remaining));
    headers.set('X-RateLimit-Reset', String(Math.ceil(bucket.resetAt / 1000)));
    return headers;
  }

  return {
    check(key: string): RateLimitCheckResult {
      const currentTime = now();
      sweepExpiredBuckets(currentTime);
      const existingBucket = buckets.get(key);
      const bucket =
        !existingBucket || existingBucket.resetAt <= currentTime
          ? { count: 0, resetAt: currentTime + windowMs }
          : existingBucket;

      bucket.count += 1;
      buckets.set(key, bucket);

      if (bucket.count > limit) {
        return {
          limited: true,
          response: Response.json(
            { error: TOO_MANY_REQUESTS_MESSAGE },
            {
              status: 429,
              headers: buildHeaders(bucket),
            },
          ),
        };
      }

      return {
        limited: false,
        headers: buildHeaders(bucket),
      };
    },
    reset() {
      buckets.clear();
    },
  };
}

const authRateLimiter = createMemoryRateLimiter();
const authLoginIpRateLimiter = createMemoryRateLimiter({
  limit: AUTH_LOGIN_IP_LIMIT,
  windowMs: AUTH_LOGIN_WINDOW_MS,
});
const authLoginCredentialRateLimiter = createMemoryRateLimiter({
  limit: AUTH_LOGIN_CREDENTIAL_LIMIT,
  windowMs: AUTH_LOGIN_WINDOW_MS,
});

const SHARED_RATE_LIMIT_TIMEOUT_MS = 2_000;

type AuthRouteLimit = {
  limiter: ReturnType<typeof createMemoryRateLimiter>;
  limit: number;
  windowMs: number;
};

function resolveAuthRouteLimit(routeKey: string): AuthRouteLimit {
  if (routeKey === 'auth:login:ip') {
    return { limiter: authLoginIpRateLimiter, limit: AUTH_LOGIN_IP_LIMIT, windowMs: AUTH_LOGIN_WINDOW_MS };
  }

  if (routeKey === 'auth:login:credential') {
    return { limiter: authLoginCredentialRateLimiter, limit: AUTH_LOGIN_CREDENTIAL_LIMIT, windowMs: AUTH_LOGIN_WINDOW_MS };
  }

  return { limiter: authRateLimiter, limit: DEFAULT_LIMIT, windowMs: DEFAULT_WINDOW_MS };
}

type SharedRateLimitConfig = { url: string; token: string };

let cachedSharedConfig: SharedRateLimitConfig | null | undefined;

// Vercel 마켓플레이스 연결은 커스텀 접두사(<prefix>_KV_REST_API_URL 등)를 붙일 수 있어
// 표준 이름이 없으면 접미사로 URL 키를 찾고, 같은 접두사의 토큰 키와 짝지어 사용한다.
function findPrefixedRestConfig(urlSuffix: string, tokenSuffix: string): SharedRateLimitConfig | null {
  for (const [key, value] of Object.entries(process.env)) {
    if (!key.endsWith(urlSuffix) || !value) {
      continue;
    }

    const prefix = key.slice(0, key.length - urlSuffix.length);
    const token = process.env[`${prefix}${tokenSuffix}`];
    if (token) {
      return { url: value, token };
    }
  }

  return null;
}

function getSharedRateLimitConfig(): SharedRateLimitConfig | null {
  if (cachedSharedConfig !== undefined) {
    return cachedSharedConfig;
  }

  cachedSharedConfig =
    findPrefixedRestConfig('UPSTASH_REDIS_REST_URL', 'UPSTASH_REDIS_REST_TOKEN') ??
    findPrefixedRestConfig('KV_REST_API_URL', 'KV_REST_API_TOKEN');

  return cachedSharedConfig;
}

function buildSharedRateLimitHeaders(limit: number, count: number, resetAt: number) {
  const headers = new Headers();
  headers.set('Cache-Control', 'no-store');
  headers.set('Retry-After', String(Math.max(1, Math.ceil((resetAt - Date.now()) / 1000))));
  headers.set('X-RateLimit-Limit', String(limit));
  headers.set('X-RateLimit-Remaining', String(Math.max(0, limit - count)));
  headers.set('X-RateLimit-Reset', String(Math.ceil(resetAt / 1000)));
  headers.set('X-RateLimit-Scope', 'shared');
  return headers;
}

// Upstash REST 파이프라인으로 고정 윈도우 카운트: INCR 후 첫 증가에만 TTL을 걸고(PEXPIRE NX) 남은 TTL을 읽는다.
async function checkSharedRateLimit(
  key: string,
  limit: number,
  windowMs: number,
  config: { url: string; token: string },
): Promise<RateLimitCheckResult | null> {
  const redisKey = `rl:${key}`;
  const response = await fetch(`${config.url}/pipeline`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${config.token}`,
      'content-type': 'application/json',
    },
    body: JSON.stringify([
      ['INCR', redisKey],
      ['PEXPIRE', redisKey, String(windowMs), 'NX'],
      ['PTTL', redisKey],
    ]),
    signal: AbortSignal.timeout(SHARED_RATE_LIMIT_TIMEOUT_MS),
  });

  if (!response.ok) {
    return null;
  }

  const results = (await response.json()) as Array<{ result?: unknown; error?: string }>;
  const count = Number(results?.[0]?.result);
  const remainingTtlMs = Number(results?.[2]?.result);
  if (!Number.isFinite(count) || !Number.isFinite(remainingTtlMs) || remainingTtlMs < 0) {
    return null;
  }

  const resetAt = Date.now() + remainingTtlMs;
  if (count > limit) {
    return {
      limited: true,
      response: Response.json(
        { error: TOO_MANY_REQUESTS_MESSAGE },
        { status: 429, headers: buildSharedRateLimitHeaders(limit, count, resetAt) },
      ),
    };
  }

  return {
    limited: false,
    headers: buildSharedRateLimitHeaders(limit, count, resetAt),
  };
}

function normalizeCredential(credential: string) {
  return credential.trim().toLowerCase();
}

export function buildAuthRateLimitKey(
  request: Pick<Request, 'headers'>,
  routeKey: string,
  options: AuthRateLimitOptions = {},
) {
  const keyParts = [routeKey, getClientIp(request)];
  const normalizedCredential = options.credential ? normalizeCredential(options.credential) : '';

  if (normalizedCredential) {
    keyParts.push(normalizedCredential);
  }

  return keyParts.join(':');
}

export async function checkAuthRateLimit(
  request: Pick<Request, 'headers'>,
  routeKey: string,
  options: AuthRateLimitOptions = {},
): Promise<RateLimitCheckResult> {
  const { limiter, limit, windowMs } = resolveAuthRouteLimit(routeKey);
  const key = buildAuthRateLimitKey(request, routeKey, options);

  const sharedConfig = getSharedRateLimitConfig();
  if (sharedConfig) {
    try {
      const sharedResult = await checkSharedRateLimit(key, limit, windowMs, sharedConfig);
      if (sharedResult) {
        return sharedResult;
      }
    } catch (error) {
      console.error('Shared rate limiter unavailable, falling back to per-instance limiter:', error);
    }
  }

  return limiter.check(key);
}

export function applyRateLimitHeaders(response: Response, headers: Headers) {
  const mergedHeaders = new Headers(response.headers);
  headers.forEach((value, key) => {
    mergedHeaders.set(key, value);
  });

  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers: mergedHeaders,
  });
}