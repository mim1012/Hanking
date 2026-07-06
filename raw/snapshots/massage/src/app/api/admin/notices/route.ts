import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { createNotice, listNotices } from '@/lib/server/communityStore';

export async function GET(request: Request) {
  try {
    await requireRole('ADMIN');
    const url = new URL(request.url);
    const search = url.searchParams.get('search') ?? url.searchParams.get('q') ?? undefined;
    return Response.json({ notices: await listNotices({ search: search?.trim() || undefined, strict: true }) });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function POST(request: Request) {
  try {
    const user = await requireRole('ADMIN');
    const body = (await request.json()) as {
      title?: string;
      content?: string;
      isPinned?: boolean;
    };

    if (!body.title?.trim() || !body.content?.trim()) {
      return Response.json({ error: '제목과 내용은 필수입니다.' }, { status: 400 });
    }

    return Response.json(
      {
        notice: await createNotice({
          title: body.title,
          content: body.content,
          isPinned: Boolean(body.isPinned),
          createdBy: user.id,
        }),
      },
      { status: 201 },
    );
  } catch (error) {
    return errorResponse(error);
  }
}
