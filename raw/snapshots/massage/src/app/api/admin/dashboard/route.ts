import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { getCachedAdminDashboardData } from '@/lib/server/communityStore';

export async function GET() {
  try {
    await requireRole('ADMIN');
    return Response.json(await getCachedAdminDashboardData());
  } catch (error) {
    return errorResponse(error);
  }
}
