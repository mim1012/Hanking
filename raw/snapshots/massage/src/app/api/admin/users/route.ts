import { UserRole, UserStatus } from '@prisma/client';
import { getSessionUser, requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { listUsersPage, updateManagedUser } from '@/lib/server/auth-store';

type UpdateUserBody = {
  name?: string;
  phone?: string | null;
  status?: 'pending' | 'approved' | 'rejected';
};

function parsePositiveInt(value: string | null, fallback: number) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : fallback;
}

function parseRole(value: string | null) {
  if (!value || value === 'all') return undefined;
  return value in UserRole ? (value as UserRole) : undefined;
}

function parseStatus(value: string | null) {
  if (!value || value === 'all') return undefined;
  if (value === 'pending') return UserStatus.PENDING;
  if (value === 'rejected') return UserStatus.REJECTED;
  if (value === 'approved') return UserStatus.APPROVED;
  return undefined;
}

function isValidStatus(value: unknown): value is NonNullable<UpdateUserBody['status']> {
  return value === 'pending' || value === 'approved' || value === 'rejected';
}

export async function GET(request: Request) {
  try {
    await requireRole('ADMIN');
    const searchParams = new URL(request.url).searchParams;
    const result = await listUsersPage({
      page: parsePositiveInt(searchParams.get('page'), 1),
      pageSize: parsePositiveInt(searchParams.get('pageSize'), 20),
      query: searchParams.get('q')?.trim() || undefined,
      role: parseRole(searchParams.get('role')),
      status: parseStatus(searchParams.get('status')),
    });

    return Response.json(result, {
      headers: {
        'Cache-Control': 'private, no-store',
      },
    });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function PATCH(request: Request) {
  try {
    const currentUser = await getSessionUser();
    if (!currentUser || currentUser.role !== 'ADMIN') {
      await requireRole('ADMIN');
    }

    const body = (await request.json()) as UpdateUserBody & { id?: string };
    const userId = body.id?.trim();
    if (!userId) {
      return Response.json({ error: '회원 ID가 필요합니다.' }, { status: 400 });
    }

    if (currentUser?.id === userId && body.status && body.status !== 'approved') {
      return Response.json({ error: '현재 로그인한 관리자 계정은 비활성화할 수 없습니다.' }, { status: 400 });
    }

    const user = await updateManagedUser(userId, {
      name: body.name,
      phone: body.phone,
      status: isValidStatus(body.status) ? body.status : undefined,
    });

    if (!user) {
      return Response.json({ error: '변경할 회원 정보가 없습니다.' }, { status: 400 });
    }

    return Response.json({ user });
  } catch (error) {
    if (error instanceof Error && error.message === 'USER_STATUS_NOT_MANAGED') {
      return Response.json({ error: '상태 변경은 업체관리자 승인 상태에만 적용됩니다.' }, { status: 400 });
    }
    if (error instanceof Error && error.message === 'USER_NAME_REQUIRED') {
      return Response.json({ error: '이름을 입력해 주세요.' }, { status: 400 });
    }

    return errorResponse(error);
  }
}
