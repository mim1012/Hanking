import type { UserRole } from '@/lib/types';

export const ADMIN_AREA_PATH = '/admin';
export const OWNER_AREA_PATH = '/owner';

export const ROLE_HOME_PATH: Record<UserRole, string> = {
  ADMIN: ADMIN_AREA_PATH,
  OWNER: '/owner/shops',
  USER: '/my',
};

const ROLE_MY_LABEL: Record<UserRole, string> = {
  ADMIN: '관리자',
  OWNER: '내업소',
  USER: 'MY',
};

export function getRoleHomeHref(role?: UserRole) {
  return role ? ROLE_HOME_PATH[role] : '/auth/login';
}

export function getMyHref(role?: UserRole) {
  return getRoleHomeHref(role);
}

export function getMyLabel(role?: UserRole) {
  return role ? ROLE_MY_LABEL[role] : ROLE_MY_LABEL.USER;
}

export function isAdminAreaPath(pathname?: string | null) {
  return pathname?.startsWith(ADMIN_AREA_PATH) ?? false;
}

export function isOwnerAreaPath(pathname?: string | null) {
  return pathname?.startsWith(OWNER_AREA_PATH) ?? false;
}

export function isOwnerAreaRole(role?: UserRole): role is 'ADMIN' | 'OWNER' {
  return role === 'ADMIN' || role === 'OWNER';
}

export function canAccessPathForRole(role: UserRole, pathname?: string | null) {
  if (!pathname) {
    return true;
  }

  if (isAdminAreaPath(pathname)) {
    return role === 'ADMIN';
  }

  if (isOwnerAreaPath(pathname)) {
    return isOwnerAreaRole(role);
  }

  return true;
}
