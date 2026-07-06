import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { listQna } from '@/lib/server/communityStore';

const ADMIN_PRIVATE_CACHE_CONTROL = 'private, no-store';

export async function GET(request: Request) {
  try {
    const user = await requireRole('ADMIN', 'OWNER');
    const url = new URL(request.url);
    const search = url.searchParams.get('search') ?? url.searchParams.get('q') ?? undefined;
    const shopId = user.role !== 'OWNER' ? (url.searchParams.get('shopId') ?? undefined) : undefined;
    const shopOwnerId = user.role === 'OWNER' ? user.id : undefined;
    const page = Number(url.searchParams.get('page') ?? Number.NaN);
    const pageSize = Number(url.searchParams.get('pageSize') ?? Number.NaN);

    const qnaList = await listQna({
      search: search?.trim() || undefined,
      shopId,
      shopOwnerId,
      viewer: { id: user.id, role: user.role },
      page: Number.isInteger(page) && page > 0 ? page : undefined,
      pageSize: Number.isInteger(pageSize) && pageSize > 0 ? pageSize : undefined,
    });

    return Response.json(
      { qnaList },
      {
        headers: {
          'Cache-Control': ADMIN_PRIVATE_CACHE_CONTROL,
        },
      },
    );
  } catch (error) {
    return errorResponse(error);
  }
}
