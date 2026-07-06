import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { updateShopPremium } from '@/lib/server/shop-store';

export async function PATCH(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    await requireRole('ADMIN');
    const { id } = await context.params;
    const body = (await request.json()) as { isPremium?: boolean; premiumOrder?: number };
    if (typeof body.isPremium !== 'boolean') {
      return Response.json({ error: '프리미엄 여부 값이 필요합니다.' }, { status: 400 });
    }

    const shop = await updateShopPremium(id, body.isPremium, body.premiumOrder);
    if (!shop) {
      return Response.json({ error: '업소를 찾을 수 없습니다.' }, { status: 404 });
    }

    return Response.json({ shop });
  } catch (error) {
    return errorResponse(error);
  }
}
