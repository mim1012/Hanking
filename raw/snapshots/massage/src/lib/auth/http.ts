import { AuthError } from '@/lib/auth/guards';
import { applyNoStoreSessionHeaders } from '@/lib/security/http';

const errorMessageMap: Record<string, string> = {
  SHOP_SLUG_IN_USE: '이미 사용 중인 슬러그입니다. 다른 URL 영문명을 입력해 주세요.',
  EMAIL_IN_USE: '이미 사용 중인 이메일입니다.',
  INVALID_CREDENTIALS: '이메일 또는 비밀번호가 올바르지 않습니다.',
  OWNER_NOT_APPROVED: '업주 계정은 관리자 승인 후 로그인할 수 있습니다.',
  DATABASE_ERROR: '데이터베이스 연결에 실패했습니다. 관리자에게 문의해 주세요.',
  'Authentication required.': '로그인한 회원만 이용할 수 있습니다.',
  'Forbidden.': '접근 권한이 없습니다.',
};

const KOREAN_TEXT_PATTERN = /[가-힣]/;

export function errorResponse(error: unknown, fallbackMessage = '예상하지 못한 서버 오류가 발생했습니다.') {
  if (error instanceof AuthError) {
    return Response.json(
      { error: errorMessageMap[error.message] ?? error.message },
      { status: error.status, headers: applyNoStoreSessionHeaders() },
    );
  }

  if (error instanceof SyntaxError) {
    return Response.json(
      { error: '요청 형식이 올바르지 않습니다.' },
      { status: 400, headers: applyNoStoreSessionHeaders() },
    );
  }

  if (error instanceof Error) {
    const normalizedMessage =
      error.message.includes('Unique constraint failed') && error.message.includes('slug')
        ? 'SHOP_SLUG_IN_USE'
        : error.message;

    // 매핑된 코드도, 한국어 사용자 메시지도 아니면 내부 정보이므로 응답에 싣지 않는다.
    if (!(normalizedMessage in errorMessageMap) && !KOREAN_TEXT_PATTERN.test(normalizedMessage)) {
      console.error('Unhandled server error:', error);
      return Response.json(
        { error: fallbackMessage },
        { status: 500, headers: applyNoStoreSessionHeaders() },
      );
    }

    const status =
      normalizedMessage === 'EMAIL_IN_USE' || normalizedMessage === 'SHOP_SLUG_IN_USE'
        ? 409
        : normalizedMessage === 'INVALID_CREDENTIALS'
          ? 401
          : normalizedMessage === 'OWNER_NOT_APPROVED'
            ? 403
            : normalizedMessage === 'DATABASE_ERROR'
              ? 503
              : 400;

    return Response.json(
      { error: errorMessageMap[normalizedMessage] ?? normalizedMessage },
      { status, headers: applyNoStoreSessionHeaders() },
    );
  }

  return Response.json({ error: fallbackMessage }, { status: 500, headers: applyNoStoreSessionHeaders() });
}
