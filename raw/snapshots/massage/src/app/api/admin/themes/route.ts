import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { addTheme, deleteTheme, listThemes, type ThemeItem } from '@/lib/server/theme-store';

export async function GET() {
  try {
    await requireRole('ADMIN');
    return Response.json({ themes: await listThemes() });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function POST(request: Request) {
  try {
    await requireRole('ADMIN');
    const body = (await request.json()) as Partial<ThemeItem>;
    if (!body.code?.trim()) {
      return Response.json({ error: 'code가 필요합니다.' }, { status: 400 });
    }
    if (!body.label?.trim()) {
      return Response.json({ error: 'label이 필요합니다.' }, { status: 400 });
    }
    const result = await addTheme({
      code: body.code,
      label: body.label,
      emoji: body.emoji ?? '',
    });
    if (!result.ok) {
      return Response.json({ error: result.error }, { status: 409 });
    }
    return Response.json({ theme: result.theme }, { status: 201 });
  } catch (error) {
    return errorResponse(error);
  }
}

export async function DELETE(request: Request) {
  try {
    await requireRole('ADMIN');
    const { code } = (await request.json()) as { code?: string };
    if (!code?.trim()) {
      return Response.json({ error: 'code가 필요합니다.' }, { status: 400 });
    }
    const result = await deleteTheme(code);
    if (!result.ok) {
      return Response.json({ error: result.error }, { status: result.status });
    }
    return Response.json({ ok: true });
  } catch (error) {
    return errorResponse(error);
  }
}
