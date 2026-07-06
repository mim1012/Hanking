const DEVELOPMENT_FALLBACK_SECRET = 'replace-this-with-a-long-random-secret';

export function getSessionSecret(env = process.env) {
  const sessionSecret = env.SESSION_SECRET?.trim();
  const nodeEnv = env.NODE_ENV ?? 'development';

  if (sessionSecret) {
    if (nodeEnv === 'production' && sessionSecret === DEVELOPMENT_FALLBACK_SECRET) {
      throw new Error('SESSION_SECRET_INSECURE');
    }

    return sessionSecret;
  }

  if (nodeEnv === 'production') {
    throw new Error('SESSION_SECRET_REQUIRED');
  }

  return DEVELOPMENT_FALLBACK_SECRET;
}

export { DEVELOPMENT_FALLBACK_SECRET };
