import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { listOwnerApprovals } from '@/lib/server/auth-store';

export async function GET() {
  try {
    await requireRole('ADMIN');
    return Response.json(await listOwnerApprovals());
  } catch (error) {
    return errorResponse(error);
  }
}
