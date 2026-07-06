import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { getSiteContent, upsertSiteContent } from '@/lib/server/communityStore';
import type { HomeSeoContent, SiteSettings } from '@/lib/types';
import {
  HOME_SEO_CONTENT_MAX_LENGTH,
  HOME_SEO_TITLE_MAX_LENGTH,
  sanitizeBoundedText,
} from '@/lib/site-content-limits';

type SiteContentPayload = SiteSettings & HomeSeoContent;

function sanitizeSiteContentPayload(body: SiteContentPayload): SiteContentPayload {
  return {
    ...body,
    siteName: body.siteName.trim(),
    siteTitle: body.siteTitle.trim(),
    siteDescription: body.siteDescription.trim(),
    heroMainText: body.heroMainText.trim(),
    heroSubText: body.heroSubText.trim(),
    contactPhone: body.contactPhone.trim(),
    footerInfo: body.footerInfo.trim(),
    section1Title: sanitizeBoundedText(body.section1Title, HOME_SEO_TITLE_MAX_LENGTH),
    section1Content: sanitizeBoundedText(body.section1Content, HOME_SEO_CONTENT_MAX_LENGTH),
    section2Title: sanitizeBoundedText(body.section2Title, HOME_SEO_TITLE_MAX_LENGTH),
    section2Content: sanitizeBoundedText(body.section2Content, HOME_SEO_CONTENT_MAX_LENGTH),
    section3Title: sanitizeBoundedText(body.section3Title, HOME_SEO_TITLE_MAX_LENGTH),
    section3Content: sanitizeBoundedText(body.section3Content, HOME_SEO_CONTENT_MAX_LENGTH),
  };
}

export async function GET() {
  try {
    await requireRole('ADMIN');
    return Response.json(await getSiteContent());
  } catch (error) {
    return errorResponse(error);
  }
}

export async function PATCH(request: Request) {
  try {
    await requireRole('ADMIN');
    const body = (await request.json()) as Partial<SiteContentPayload>;

    const requiredFields: Array<keyof SiteContentPayload> = [
      'siteName',
      'siteTitle',
      'siteDescription',
      'heroMainText',
      'heroSubText',
      'contactPhone',
      'footerInfo',
      'section1Title',
      'section1Content',
      'section2Title',
      'section2Content',
      'section3Title',
      'section3Content',
    ];

    if (requiredFields.some((field) => !body[field]?.trim())) {
      return Response.json({ error: '사이트 설정의 모든 항목을 입력해 주세요.' }, { status: 400 });
    }

    const sanitizedBody = sanitizeSiteContentPayload(body as SiteContentPayload);

    return Response.json(
      await upsertSiteContent(sanitizedBody),
    );
  } catch (error) {
    return errorResponse(error);
  }
}
