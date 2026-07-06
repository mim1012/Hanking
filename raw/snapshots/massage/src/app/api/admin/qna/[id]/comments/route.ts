import { assertOwnershipOrAdmin, requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { createQnaComment, getQnaShopOwnerId } from '@/lib/server/communityStore';

export async function POST(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { id } = await context.params;
    const body = (await request.json()) as { content?: string };

    if (!body.content?.trim()) {
      return Response.json({ error: '댓글 내용은 필수입니다.' }, { status: 400 });
    }

    const qnaAccess = await getQnaShopOwnerId(id);
    if (!qnaAccess.exists) {
      return Response.json({ error: 'Q&A 항목을 찾을 수 없습니다.' }, { status: 404 });
    }

    assertOwnershipOrAdmin(user, qnaAccess.ownerId);

    const qna = await createQnaComment(
      id,
      {
        content: body.content,
        userId: user.id,
        authorName: user.name,
        role: user.role === 'OWNER' ? 'OWNER' : 'ADMIN',
      },
      { id: user.id, role: user.role },
    );

    if (!qna) {
      return Response.json({ error: 'Q&A 항목을 찾을 수 없습니다.' }, { status: 404 });
    }

    return Response.json({ qna }, { status: 201 });
  } catch (error) {
    return errorResponse(error);
  }
}
