import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { getPremiumBoardData, updatePremiumOrder } from '@/lib/server/communityStore';

export async function GET() {
  try {
    await requireRole('ADMIN');
    return Response.json(await getPremiumBoardData());
  } catch (error) {
    return errorResponse(error);
  }
}

export async function PATCH(request: Request) {
  try {
    await requireRole('ADMIN');
    const body = (await request.json()) as { orderedIds?: string[]; visibilityById?: Record<string, boolean> };
    if (!Array.isArray(body.orderedIds)) {
      return Response.json({ error: 'orderedIds must be an array.' }, { status: 400 });
    }

    return Response.json(await updatePremiumOrder(body.orderedIds, body.visibilityById));
  } catch (error) {
    return errorResponse(error);
  }
}
