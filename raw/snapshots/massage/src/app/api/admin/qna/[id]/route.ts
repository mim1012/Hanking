import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { prisma } from '@/lib/db/prisma';
import { withDatabaseRetry } from '@/lib/db/retry';
import { deleteManagedQna, updateQna } from '@/lib/server/communityStore';

type Context = {
  params: Promise<{ id: string }>;
};

export async function PATCH(request: Request, context: Context) {
  try {
    const user = await requireRole('ADMIN');
    const { id } = await context.params;

    const existing = await withDatabaseRetry(() =>
      prisma.qnA.findUnique({
        where: { id },
        include: { shop: { select: { ownerId: true } } },
      }),
    );

    if (!existing) {
      return Response.json({ error: 'Q&A를 찾을 수 없습니다.' }, { status: 404 });
    }

    if (user.role !== 'ADMIN' && existing.shop?.ownerId !== user.id) {
      return Response.json({ error: '권한이 없습니다.' }, { status: 403 });
    }

    const body = (await request.json()) as { question?: string };
    if (!body.question?.trim()) {
      return Response.json({ error: '질문 내용은 필수입니다.' }, { status: 400 });
    }

    const updated = await updateQna(id, { question: body.question.trim() }, { id: user.id, role: user.role });
    if (!updated) {
      return Response.json({ error: 'Q&A를 수정하지 못했습니다.' }, { status: 404 });
    }

    return Response.json({ qna: updated });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function DELETE(_: Request, context: Context) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { id } = await context.params;
    const deleted = await deleteManagedQna(user, id);

    if (!deleted) {
      return Response.json({ error: 'Q&A를 찾을 수 없거나 권한이 없습니다.' }, { status: 404 });
    }

    return new Response(null, { status: 204 });
  } catch (error) {
    return errorResponse(error);
  }
}
