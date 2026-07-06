import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { deletePartnershipInquiry, updatePartnershipInquiryStatus } from '@/lib/server/communityStore';
import type { PartnershipInquiry } from '@/lib/types';

type Context = {
  params: Promise<{ id: string }>;
};

export async function PATCH(request: Request, context: Context) {
  try {
    await requireRole('ADMIN');
    const { id } = await context.params;
    const body = (await request.json()) as { status?: PartnershipInquiry['status'] };

    if (!body.status || !['pending', 'contacted', 'completed'].includes(body.status)) {
      return Response.json({ error: '유효한 상태값이 필요합니다.' }, { status: 400 });
    }

    const inquiry = await updatePartnershipInquiryStatus(id, body.status);
    if (!inquiry) {
      return Response.json({ error: '제휴 문의를 찾을 수 없습니다.' }, { status: 404 });
    }

    return Response.json({ inquiry });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function DELETE(_: Request, context: Context) {
  try {
    await requireRole('ADMIN');
    const { id } = await context.params;
    const deleted = await deletePartnershipInquiry(id);
    if (!deleted) {
      return Response.json({ error: '제휴 문의를 찾을 수 없습니다.' }, { status: 404 });
    }

    return new Response(null, { status: 204 });
  } catch (error) {
    return errorResponse(error);
  }
}
