import { assertOwnershipOrAdmin, requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { normalizeShopInputForSave } from '@/lib/server/admin-shop-access';
import { deleteAdminShop, getAdminShopById, updateAdminShop } from '@/lib/server/communityStore';
import type { Shop } from '@/lib/types';

export async function GET(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { id } = await context.params;
    const shop = await getAdminShopById(id);
    if (!shop) {
      return Response.json({ error: '업소를 찾을 수 없습니다.' }, { status: 404 });
    }

    assertOwnershipOrAdmin(user, shop.ownerId);

    return Response.json({ shop });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function DELETE(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { id } = await context.params;

    const existingShop = await getAdminShopById(id);
    if (!existingShop) {
      return Response.json({ error: '업소를 찾을 수 없습니다.' }, { status: 404 });
    }

    assertOwnershipOrAdmin(user, existingShop.ownerId);

    const deleted = await deleteAdminShop(id, user.role === 'OWNER' ? { ownerId: user.id } : undefined);
    if (!deleted) {
      return Response.json({ error: '업소를 찾을 수 없습니다.' }, { status: 404 });
    }

    return Response.json({ ok: true });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function PATCH(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { id } = await context.params;
    const body = (await request.json()) as { shop?: Shop };
    if (!body.shop) {
      return Response.json({ error: '업소 정보가 필요합니다.' }, { status: 400 });
    }

    const existingShop = await getAdminShopById(id);
    if (!existingShop) {
      return Response.json({ error: '업소를 찾을 수 없습니다.' }, { status: 404 });
    }

    assertOwnershipOrAdmin(user, existingShop.ownerId);

    const shopInput = normalizeShopInputForSave(user, body.shop, existingShop);

    const shop = await updateAdminShop(id, shopInput, user.role === 'OWNER' ? { ownerId: user.id } : undefined);
    if (!shop) {
      return Response.json({ error: '업소를 찾을 수 없습니다.' }, { status: 404 });
    }

    return Response.json({ shop });
  } catch (error) {
    return errorResponse(error);
  }
}
