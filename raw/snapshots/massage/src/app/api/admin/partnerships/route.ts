import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { listPartnershipInquiries } from '@/lib/server/communityStore';

export async function GET() {
  try {
    await requireRole('ADMIN');
    return Response.json({ inquiries: await listPartnershipInquiries() });
  } catch (error) {
    return errorResponse(error);
  }
}
