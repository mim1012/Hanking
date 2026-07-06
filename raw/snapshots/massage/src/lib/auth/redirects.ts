import type { UserRole } from '@/lib/types';
import { canAccessPathForRole, getRoleHomeHref } from '@/lib/auth/navigation';

export function normalizeSafeRedirectPath(value?: string | null) {
  const trimmed = value?.trim();
  if (!trimmed || !trimmed.startsWith('/') || trimmed.startsWith('//') || trimmed.startsWith('/\\') || trimmed.includes('\\')) {
    return null;
  }

  try {
    const parsed = new URL(trimmed, 'https://massage.local');
    if (parsed.origin !== 'https://massage.local') {
      return null;
    }

    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return null;
  }
}

export function getPostLoginRedirect(role: UserRole, redirectTo?: string | null) {
  const safeRedirectPath = normalizeSafeRedirectPath(redirectTo);
  if (safeRedirectPath && canAccessPathForRole(role, safeRedirectPath)) {
    return safeRedirectPath;
  }

  if (role === 'OWNER') {
    return getRoleHomeHref(role);
  }

  // ADMIN and USER default to home page
  return '/';
}
