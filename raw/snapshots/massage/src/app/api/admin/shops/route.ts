import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { normalizeShopInputForSave } from '@/lib/server/admin-shop-access';
import { createAdminShop, listManagedShopOptions, listManagedShops, listManagedShopsPage } from '@/lib/server/communityStore';
import type { Shop } from '@/lib/types';

const ADMIN_PRIVATE_CACHE_CONTROL = 'private, no-store';

export async function GET(request: Request) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const { searchParams } = new URL(request.url);
    const filters = {
      region: searchParams.get('region') ?? undefined,
      q: searchParams.get('q') ?? undefined,
    };
    const view = searchParams.get('view');
    const pageParam = searchParams.get('page');
    if (pageParam !== null) {
      const result = await listManagedShopsPage(user, {
        ...filters,
        page: Number(pageParam) || 1,
        pageSize: Number(searchParams.get('pageSize')) || 20,
      });
      return Response.json(result, { headers: { 'Cache-Control': ADMIN_PRIVATE_CACHE_CONTROL } });
    }
    return Response.json(
      { shops: view === 'options' ? await listManagedShopOptions(user, filters) : await listManagedShops(user, filters) },
      { headers: { 'Cache-Control': ADMIN_PRIVATE_CACHE_CONTROL } },
    );
  } catch (error) {
    return errorResponse(error);
  }
}

export async function POST(request: Request) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const body = (await request.json()) as { shop?: Shop };
    if (!body.shop) {
      return Response.json({ error: '업소 정보가 필요합니다.' }, { status: 400 });
    }

    if (user.role === 'OWNER' && body.shop.ownerId && body.shop.ownerId !== user.id) {
      return Response.json({ error: 'Forbidden.' }, { status: 403 });
    }

    const shopInput = normalizeShopInputForSave(user, body.shop);

    return Response.json({ shop: await createAdminShop(shopInput) }, { status: 201 });
  } catch (error) {
    return errorResponse(error);
  }
}
