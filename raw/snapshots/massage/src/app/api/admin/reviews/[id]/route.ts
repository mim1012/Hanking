import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { prisma } from '@/lib/db/prisma';
import { deleteManagedReview, setReviewHiddenState, updateReview } from '@/lib/server/communityStore';
import { withDatabaseRetry } from '@/lib/db/retry';

type Context = {
  params: Promise<{ id: string }>;
};

export async function PATCH(request: Request, context: Context) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { id } = await context.params;
    const existing = await withDatabaseRetry(() =>
      prisma.review.findUnique({
        where: { id },
        include: { shop: { select: { ownerId: true } } },
      }),
    );

    if (!existing) {
      return Response.json({ error: '리뷰를 찾을 수 없습니다.' }, { status: 404 });
    }

    if (user.role !== 'ADMIN' && existing.shop.ownerId !== user.id) {
      return Response.json({ error: '권한이 없습니다.' }, { status: 403 });
    }

    const body = (await request.json()) as { rating?: number; content?: string; authorName?: string; isHidden?: boolean };
    if (user.role !== 'ADMIN' && (body.rating !== undefined || body.content !== undefined || body.authorName !== undefined)) {
      return Response.json({ error: '점주는 리뷰 숨김 상태만 변경할 수 있습니다.' }, { status: 403 });
    }


    if (body.isHidden !== undefined && body.rating === undefined && body.content === undefined && body.authorName === undefined) {
      const toggled = await setReviewHiddenState(user, id, body.isHidden);
      if (!toggled) {
        return Response.json({ error: '리뷰를 찾을 수 없거나 권한이 없습니다.' }, { status: 404 });
      }
      return Response.json({ review: toggled });
    }

    if (body.rating !== undefined && (!Number.isInteger(body.rating) || body.rating < 1 || body.rating > 5)) {
      return Response.json({ error: '평점은 1점부터 5점 사이여야 합니다.' }, { status: 400 });
    }
    if (body.content !== undefined && !body.content.trim()) {
      return Response.json({ error: '리뷰 내용을 입력해주세요.' }, { status: 400 });
    }
    if (body.authorName !== undefined && !body.authorName.trim()) {
      return Response.json({ error: '작성자 이름을 입력해주세요.' }, { status: 400 });
    }

    if (user.role !== 'ADMIN') {
      return Response.json({ error: '권한이 없습니다.' }, { status: 403 });
    }

    const updated = await updateReview(id, body);
    if (!updated) {
      return Response.json({ error: '리뷰를 수정하지 못했습니다.' }, { status: 404 });
    }

    return Response.json({ review: updated });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function DELETE(_: Request, context: Context) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { id } = await context.params;
    const deleted = await deleteManagedReview(user, id);

    if (!deleted) {
      return Response.json({ error: '리뷰를 찾을 수 없거나 권한이 없습니다.' }, { status: 404 });
    }

    return new Response(null, { status: 204 });
  } catch (error) {
    return errorResponse(error);
  }
}
