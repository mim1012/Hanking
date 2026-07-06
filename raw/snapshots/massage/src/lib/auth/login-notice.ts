export const PENDING_APPROVAL_LOGIN_NOTICE = 'pending-approval';
export const OWNER_PENDING_APPROVAL_ERROR = '업주 계정은 관리자 승인 후 로그인할 수 있습니다.';

const pendingApprovalNotice = {
  tone: 'info' as const,
  message: '입점 신청이 접수되었습니다. 관리자 승인 후 로그인할 수 있습니다.',
};

export function getPendingApprovalLoginHref() {
  return `/auth/login?notice=${PENDING_APPROVAL_LOGIN_NOTICE}`;
}

export function getLoginNotice(notice?: string | null, error?: string | null) {
  if (notice === PENDING_APPROVAL_LOGIN_NOTICE || error === OWNER_PENDING_APPROVAL_ERROR) {
    return pendingApprovalNotice;
  }

  return null;
}
