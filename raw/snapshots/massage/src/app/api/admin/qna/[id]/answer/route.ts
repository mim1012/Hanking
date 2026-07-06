import { assertOwnershipOrAdmin, requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { answerQna, getQnaShopOwnerId } from '@/lib/server/communityStore';

export async function PATCH(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { id } = await context.params;
    const body = (await request.json()) as { answer?: string; comment?: string };
    const nextComment = body.comment?.trim() || body.answer?.trim();

    if (!nextComment) {
      return Response.json({ error: '답변 내용은 필수입니다.' }, { status: 400 });
    }

    const qnaAccess = await getQnaShopOwnerId(id);
    if (!qnaAccess.exists) {
      return Response.json({ error: 'Q&A 항목을 찾을 수 없습니다.' }, { status: 404 });
    }

    assertOwnershipOrAdmin(user, qnaAccess.ownerId);

    const qna = await answerQna(
      id,
      nextComment,
      user.id,
      user.name,
      user.role === 'OWNER' ? 'OWNER' : 'ADMIN',
      { id: user.id, role: user.role },
    );
    if (!qna) {
      return Response.json({ error: 'Q&A 항목을 찾을 수 없습니다.' }, { status: 404 });
    }

    return Response.json({ qna });
  } catch (error) {
    return errorResponse(error);
  }
}
