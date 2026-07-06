import { cache } from 'react';
import { type User, type UserRole } from '@/lib/types';
import { getSessionCookie } from '@/lib/auth/session';
import { getUserBySessionToken } from '@/lib/server/auth-store';

export class AuthError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

const getCachedUserBySessionToken = cache(async (token: string | undefined) => getUserBySessionToken(token));

export async function getSessionUser() {
  const token = await getSessionCookie();
  return await getCachedUserBySessionToken(token ?? undefined);
}

export async function getOptionalSessionUser() {
  try {
    return await getSessionUser();
  } catch (error) {
    if (error instanceof Error && error.message === 'DATABASE_ERROR') {
      return null;
    }

    throw error;
  }
}

export async function requireUser() {
  const user = await getSessionUser();
  if (!user) {
    throw new AuthError('Authentication required.', 401);
  }

  return user;
}

export async function requireRole(...roles: UserRole[]) {
  const user = await requireUser();
  if (!roles.includes(user.role)) {
    throw new AuthError('Forbidden.', 403);
  }

  return user;
}

export function assertOwnershipOrAdmin(
  user: Pick<User, 'id' | 'role'>,
  resourceOwnerId: string | null | undefined,
) {
  if (user.role === 'ADMIN') {
    return;
  }

  if (user.role !== 'OWNER' || !resourceOwnerId || resourceOwnerId !== user.id) {
    throw new AuthError('Forbidden.', 403);
  }
}
