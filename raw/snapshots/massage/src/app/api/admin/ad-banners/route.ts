import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { listAllAdBanners, upsertAdBanner } from '@/lib/server/ad-banner-store';

export async function GET() {
  try {
    await requireRole('ADMIN');
    return Response.json({ banners: await listAllAdBanners() });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function PATCH(request: Request) {
  try {
    await requireRole('ADMIN');
    const body = (await request.json()) as {
      slot?: string;
      imageUrl?: string;
      linkUrl?: string | null;
      isActive?: boolean;
    };

    if (!body.slot?.trim()) {
      return Response.json({ error: '슬롯이 필요합니다.' }, { status: 400 });
    }

    const banner = await upsertAdBanner(body.slot.trim(), {
      imageUrl: body.imageUrl,
      linkUrl: body.linkUrl,
      isActive: body.isActive,
    });

    return Response.json({ banner });
  } catch (error) {
    if (error instanceof Error && error.message === 'INVALID_AD_SLOT') {
      return Response.json({ error: '유효하지 않은 광고 슬롯입니다.' }, { status: 400 });
    }
    return errorResponse(error);
  }
}
