import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { DEFAULT_LEGAL_DOCUMENTS, type EditableLegalDocument, type LegalDocumentSlug } from '@/lib/legal-documents';
import { getAllLegalDocuments, upsertLegalDocument } from '@/lib/server/legal-documents';
import { invalidatePublicLegalDocument } from '@/lib/server/public-legal-documents';

type LegalDocumentsPayload = {
  slug?: LegalDocumentSlug;
} & Partial<EditableLegalDocument>;

export async function GET() {
  try {
    await requireRole('ADMIN');
    return Response.json(await getAllLegalDocuments());
  } catch (error) {
    return errorResponse(error);
  }
}

export async function PATCH(request: Request) {
  try {
    await requireRole('ADMIN');
    const body = (await request.json()) as LegalDocumentsPayload;

    if (
      body.slug !== 'privacy' &&
      body.slug !== 'terms' &&
      body.slug !== 'youth' &&
      body.slug !== 'ad' &&
      body.slug !== 'mobile' &&
      body.slug !== 'partnership'
    ) {
      return Response.json({ error: '수정할 문서 종류를 확인해 주세요.' }, { status: 400 });
    }

    const fallback = DEFAULT_LEGAL_DOCUMENTS[body.slug];
    const payload: EditableLegalDocument = {
      eyebrow: body.eyebrow ?? fallback.eyebrow,
      title: body.title ?? fallback.title,
      description: body.description ?? fallback.description,
      note: body.note ?? fallback.note,
      body: body.body ?? fallback.body,
    };

    if (!payload.eyebrow.trim() || !payload.title.trim() || !payload.description.trim() || !payload.body.trim()) {
      return Response.json({ error: '문서 제목, 설명, 본문은 비워둘 수 없습니다.' }, { status: 400 });
    }

    const document = await upsertLegalDocument(body.slug, payload);
    invalidatePublicLegalDocument(body.slug);
    return Response.json(document);
  } catch (error) {
    return errorResponse(error);
  }
}
