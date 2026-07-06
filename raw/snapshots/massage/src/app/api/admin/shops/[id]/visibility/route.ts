import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { updateShopVisibility } from '@/lib/server/shop-store';

export async function PATCH(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    await requireRole('ADMIN');
    const { id } = await context.params;
    const body = (await request.json()) as { isVisible?: boolean };
    if (typeof body.isVisible !== 'boolean') {
      return Response.json({ error: '노출 여부 값이 필요합니다.' }, { status: 400 });
    }

    const shop = await updateShopVisibility(id, body.isVisible);
    if (!shop) {
      return Response.json({ error: '업소를 찾을 수 없습니다.' }, { status: 404 });
    }

    return Response.json({ shop });
  } catch (error) {
    return errorResponse(error);
  }
}
