import { revalidatePath, revalidateTag, unstable_cache } from 'next/cache';
import {
  Prisma,
  QnaCommentRole,
  QnaStatus,
  UserRole as PrismaUserRole,
  UserStatus,
  type Notice as DbNotice,
  type PartnershipInquiry as DbPartnershipInquiry,
  type Review as DbReview,
  type SiteSettings as DbSiteSettings,
} from '@prisma/client';
import type {
  HomeSeoContent,
  Notice,
  PartnershipInquiry,
  QnA,
  QnAComment,
  Review,
  Shop,
  SiteSettings,
  UserRole,
} from '@/lib/types';
import type { AdminDashboardData, AdminShopListItem, AdminShopOption, AdminStatsData, PremiumBoardData } from '@/lib/communityTypes';
import { prisma } from '@/lib/db/prisma';
import { withDatabaseRetry, isRecordNotFoundError } from '@/lib/db/retry';
import { clampPage, getTotalPages } from '@/lib/pagination';
import { getRegionVariants } from '@/lib/region-compat';
import {
  normalizeHomeSeo,
  normalizeSiteSettings,
} from '@/lib/site-content-defaults';
import { invalidatePublicShopCaches, mapShop, shopInclude } from '@/lib/server/shop-store';
import {
  HOME_SEO_CONTENT_MAX_LENGTH,
  HOME_SEO_TITLE_MAX_LENGTH,
  sanitizeBoundedText,
} from '@/lib/site-content-limits';

const managedShopListSelect = {
  id: true,
  name: true,
  region: true,
  regionLabel: true,
  subRegion: true,
  subRegionLabel: true,
  theme: true,
  themeLabel: true,
  phone: true,
  isVisible: true,
  isPremium: true,
  premiumOrder: true,
  ownerId: true,
  createdAt: true,
  updatedAt: true,
} satisfies Prisma.ShopSelect;

const managedShopOptionSelect = {
  id: true,
  name: true,
} satisfies Prisma.ShopSelect;

const adminShopDetailInclude = {
  images: true,
  courses: true,
} satisfies Prisma.ShopInclude;

type ManagedShopListRecord = Prisma.ShopGetPayload<{
  select: typeof managedShopListSelect;
}>;

type ManagedShopOptionRecord = Prisma.ShopGetPayload<{
  select: typeof managedShopOptionSelect;
}>;

const SITE_SETTINGS_ID = 'default';
const PUBLIC_SITE_CONTENT_CACHE_TAG = 'public-site-content';

const ADMIN_DASHBOARD_CACHE_TAG = 'admin-dashboard';
const PUBLIC_BOARD_CACHE_TAG = 'public-board';
const MANAGED_SHOPS_CACHE_TAG = 'managed-shops';
const MANAGED_QNA_CACHE_TAG = 'managed-qna';

type ManagedShopPageResult = {
  shops: AdminShopListItem[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
};
type ManagedReviewItem = Review & { shopRegionLabel: string };

const managedShopPageInFlight = new Map<string, Promise<ManagedShopPageResult>>();
const managedShopsInFlight = new Map<string, Promise<AdminShopListItem[]>>();
const managedShopOptionsInFlight = new Map<string, Promise<AdminShopOption[]>>();
const managedQnaInFlight = new Map<string, Promise<QnA[]>>();
const managedReviewsInFlight = new Map<string, Promise<ManagedReviewItem[]>>();

let cachedPublicSiteContentPromise:
  | Promise<{
      siteSettings: SiteSettings;
      homeSeo: HomeSeoContent;
    } | null>
  | null = null;

function getInFlight<T>(store: Map<string, Promise<T>>, key: string, load: () => Promise<T>) {
  const activeRequest = store.get(key);
  if (activeRequest) {
    return activeRequest;
  }

  const request = load().finally(() => {
    store.delete(key);
  });
  store.set(key, request);
  return request;
}

const getPersistentPublicSiteContent = unstable_cache(
  async () => {
    const loaded = await loadSiteContentRecord();
    return loaded?.content ?? null;
  },
  [PUBLIC_SITE_CONTENT_CACHE_TAG],
  {
    revalidate: 60,
    tags: [PUBLIC_SITE_CONTENT_CACHE_TAG],
  },
);

function isMissingNextCacheContextError(error: unknown) {
  if (!(error instanceof Error)) {
    return false;
  }

  return (
    error.message.includes('incrementalCache missing') ||
    error.message.includes('static generation store missing')
  );
}

async function readPublicSiteContentWithFallback() {
  try {
    return await getPersistentPublicSiteContent();
  } catch (error) {
    if (!isMissingNextCacheContextError(error)) {
      throw error;
    }

    const loaded = await loadSiteContentRecord();
    return loaded?.content ?? null;
  }
}

function safeRevalidateTag(tag: string) {
  try {
    revalidateTag(tag, 'max');
  } catch (error) {
    if (!isMissingNextCacheContextError(error)) {
      throw error;
    }
  }
}

function invalidateManagedAdminCaches() {
  safeRevalidateTag(MANAGED_SHOPS_CACHE_TAG);
  safeRevalidateTag(MANAGED_QNA_CACHE_TAG);
}

function invalidatePublicBoardCaches() {
  safeRevalidateTag(PUBLIC_BOARD_CACHE_TAG);
  safeRevalidateTag(ADMIN_DASHBOARD_CACHE_TAG);
  safeRevalidateTag(MANAGED_QNA_CACHE_TAG);
}

function mapManagedShopRecordForAdmin(shop: ManagedShopListRecord): AdminShopListItem {
  return {
    id: shop.id,
    name: shop.name,
    region: shop.region,
    regionLabel: shop.regionLabel,
    subRegion: shop.subRegion ?? undefined,
    subRegionLabel: shop.subRegionLabel ?? undefined,
    theme: shop.theme,
    themeLabel: shop.themeLabel,
    phone: shop.phone,
    isVisible: shop.isVisible,
    isPremium: shop.isPremium,
    premiumOrder: shop.premiumOrder ?? undefined,
    ownerId: shop.ownerId ?? undefined,
    createdAt: shop.createdAt.toISOString(),
    updatedAt: shop.updatedAt.toISOString(),
  };
}

function mapManagedShopRecordToOption(shop: ManagedShopOptionRecord): AdminShopOption {
  return {
    id: shop.id,
    name: shop.name,
  };
}


type ViewerContext = {
  id: string;
  role: UserRole;
};

type QnaRecord = Prisma.QnAGetPayload<{
  include: {
    shop: {
      select: {
        ownerId: true;
        name: true;
        regionLabel: true;
      };
    };
    comments: {
      orderBy: {
        createdAt: 'asc';
      };
    };
  };
}>;

type LegacyQnaRecord = Prisma.QnAGetPayload<{
  include: {
    shop: {
      select: {
        ownerId: true;
        name: true;
        regionLabel: true;
      };
    };
  };
}> & {
  comments: [];
};

type BoardLandingQnaRecord = Prisma.QnAGetPayload<{
  select: {
    id: true;
    userId: true;
    shopId: true;
    question: true;
    authorName: true;
    status: true;
    createdAt: true;
    shop: {
      select: {
        ownerId: true;
        name: true;
        regionLabel: true;
      };
    };
    comments: {
      select: {
        content: true;
        createdAt: true;
      };
      orderBy: {
        createdAt: 'desc';
      };
      take: 1;
    };
    _count: {
      select: {
        comments: true;
      };
    };
  };
}>;

type LegacyBoardLandingQnaRecord = Prisma.QnAGetPayload<{
  select: {
    id: true;
    userId: true;
    shopId: true;
    question: true;
    authorName: true;
    status: true;
    createdAt: true;
    shop: {
      select: {
        ownerId: true;
        name: true;
        regionLabel: true;
      };
    };
  };
}>;

function buildContainsFilter(value: string) {
  return {
    contains: value,
    mode: Prisma.QueryMode.insensitive,
  } satisfies Prisma.StringFilter;
}

function mapNotice(notice: DbNotice): Notice {
  return {
    id: notice.id,
    title: notice.title,
    content: notice.content,
    isPinned: notice.isPinned,
    createdAt: notice.createdAt.toISOString(),
  };
}

function mapQnaComment(comment: QnaRecord['comments'][number]): QnAComment {
  return {
    id: comment.id,
    qnaId: comment.qnaId,
    authorName: comment.authorName,
    role: comment.role,
    authorRole: comment.role,
    content: comment.content,
    createdAt: comment.createdAt.toISOString(),
  };
}

function isQnaCommentStorageUnavailable(error: unknown) {
  if (!(error instanceof Error)) {
    return false;
  }

  const message = error.message.toLowerCase();
  return message.includes('qna_comments') || message.includes('comments') || message.includes('column') || message.includes('relation');
}

async function loadLegacyQnaRecords(
  where: Prisma.QnAWhereInput,
  pagination?: {
    skip?: number;
    take?: number;
  },
) {
  const entries = await withDatabaseRetry(() =>
    prisma.qnA.findMany({
      where,
      include: {
        shop: {
          select: {
            ownerId: true,
            name: true,
            regionLabel: true,
          },
        },
      },
      orderBy: { createdAt: 'desc' },
      ...(typeof pagination?.skip === 'number' ? { skip: pagination.skip } : {}),
      ...(typeof pagination?.take === 'number' ? { take: pagination.take } : {}),
    }),
  );

  return entries.map((entry) => ({ ...entry, comments: [] as [] })) satisfies LegacyQnaRecord[];
}

function canCommentOnQna(entry: { shop?: { ownerId: string | null } | null }, viewer?: ViewerContext) {
  if (!viewer) {
    return false;
  }

  if (viewer.role === 'ADMIN') {
    return true;
  }

  return viewer.role === 'OWNER' && Boolean(entry.shop?.ownerId) && entry.shop?.ownerId === viewer.id;
}

function canManagePublicQna(
  entry: { userId: string | null; status: QnaStatus },
  viewer?: ViewerContext,
  commentCount = 0,
) {
  if (!viewer) {
    return false;
  }

  if (viewer.role !== 'USER' || viewer.id !== entry.userId) {
    return false;
  }

  return entry.status === QnaStatus.OPEN && commentCount === 0;
}

function mapQna(entry: QnaRecord | LegacyQnaRecord, viewer?: ViewerContext): QnA {
  const comments = entry.comments.map(mapQnaComment);
  const latestComment = comments.at(-1);

  return {
    id: entry.id,
    canManage: canManagePublicQna(entry, viewer, comments.length),
    shopId: entry.shopId ?? undefined,
    shopName: entry.shop?.name ?? undefined,
    shopRegionLabel: entry.shop?.regionLabel ?? undefined,
    question: entry.question,
    answer: latestComment?.content,
    authorName: entry.authorName,
    isAnswered: entry.status === QnaStatus.ANSWERED || comments.length > 0,
    canComment: canCommentOnQna(entry, viewer),
    commentCount: comments.length,
    latestCommentAt: latestComment?.createdAt,
    latestCommentPreview: latestComment?.content,
    comments,
    createdAt: entry.createdAt.toISOString(),
  };
}

function mapBoardLandingQna(entry: BoardLandingQnaRecord | LegacyBoardLandingQnaRecord, viewer?: ViewerContext): QnA {
  const latestComment = 'comments' in entry ? entry.comments[0] : undefined;
  const commentCount = '_count' in entry ? entry._count.comments : 0;

  return {
    id: entry.id,
    canManage: canManagePublicQna(entry, viewer, commentCount),
    shopId: entry.shopId ?? undefined,
    shopName: entry.shop?.name ?? undefined,
    shopRegionLabel: entry.shop?.regionLabel ?? undefined,
    question: entry.question,
    answer: latestComment?.content,
    authorName: entry.authorName,
    isAnswered: entry.status === QnaStatus.ANSWERED || commentCount > 0,
    canComment: canCommentOnQna(entry, viewer),
    commentCount,
    latestCommentAt: latestComment?.createdAt.toISOString(),
    latestCommentPreview: latestComment?.content,
    comments: [],
    createdAt: entry.createdAt.toISOString(),
  };
}

function mapReview(review: DbReview & { shop: { name: string } }): Review {
  return {
    id: review.id,
    shopId: review.shopId,
    shopName: review.shop.name,
    authorName: review.authorName,
    rating: review.rating,
    content: review.content,
    isHidden: review.isHidden,
    userId: review.userId ?? undefined,
    createdAt: review.createdAt.toISOString(),
  };
}

function mapPartnershipInquiry(entry: DbPartnershipInquiry): PartnershipInquiry {
  const statusMap = {
    PENDING: 'pending',
    CONTACTED: 'contacted',
    COMPLETED: 'completed',
  } as const;

  return {
    id: entry.id,
    shopName: entry.shopName,
    region: entry.region,
    subRegion: entry.subRegion,
    theme: entry.theme,
    contactName: entry.contactName,
    phone: entry.phone,
    kakaoId: entry.kakaoId ?? undefined,
    message: entry.message,
    status: statusMap[entry.status],
    createdAt: entry.createdAt.toISOString(),
  };
}

function mapPartnershipStatus(status: PartnershipInquiry['status']) {
  switch (status) {
    case 'contacted':
      return 'CONTACTED';
    case 'completed':
      return 'COMPLETED';
    case 'pending':
    default:
      return 'PENDING';
  }
}

function mapSiteSettings(record: DbSiteSettings) {
  const siteSettings = normalizeSiteSettings({
    siteName: record.siteName,
    siteTitle: record.siteTitle,
    siteDescription: record.siteDescription,
    heroMainText: record.heroMainText,
    heroSubText: record.heroSubText,
    contactPhone: record.contactPhone,
    footerInfo: record.footerInfo,
  });

  const homeSeo = normalizeHomeSeo({
    section1Title: record.seoSection1Title,
    section1Content: record.seoSection1Content,
    section2Title: record.seoSection2Title,
    section2Content: record.seoSection2Content,
    section3Title: record.seoSection3Title,
    section3Content: record.seoSection3Content,
  });

  return { siteSettings, homeSeo };
}

function hasLegacySiteContent(
  record: DbSiteSettings,
  content: {
    siteSettings: SiteSettings;
    homeSeo: HomeSeoContent;
  },
) {
  return (
    record.siteName !== content.siteSettings.siteName ||
    record.siteTitle !== content.siteSettings.siteTitle ||
    record.siteDescription !== content.siteSettings.siteDescription ||
    record.heroMainText !== content.siteSettings.heroMainText ||
    record.heroSubText !== content.siteSettings.heroSubText ||
    record.contactPhone !== content.siteSettings.contactPhone ||
    record.footerInfo !== content.siteSettings.footerInfo ||
    record.seoSection1Title !== content.homeSeo.section1Title ||
    record.seoSection1Content !== content.homeSeo.section1Content ||
    record.seoSection2Title !== content.homeSeo.section2Title ||
    record.seoSection2Content !== content.homeSeo.section2Content ||
    record.seoSection3Title !== content.homeSeo.section3Title ||
    record.seoSection3Content !== content.homeSeo.section3Content
  );
}

async function loadSiteContentRecord() {
  const record = await withDatabaseRetry(() =>
    prisma.siteSettings.findUnique({
      where: { id: SITE_SETTINGS_ID },
    }),
  );

  if (!record) {
    return null;
  }

  return {
    record,
    content: mapSiteSettings(record),
  };
}

function parseInteger(value: string) {
  const parsed = Number.parseInt(value.replace(/\D/g, ''), 10);
  return Number.isFinite(parsed) ? parsed : 0;
}
function createSlugBase(input: Pick<Shop, 'name' | 'region' | 'theme'>) {
  const fromName = input.name
    .trim()
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');

  return fromName || `${input.region}-${input.theme}-shop`;
}

async function createUniqueShopSlug(input: Pick<Shop, 'name' | 'region' | 'theme'>) {
  const base = createSlugBase(input);
  let candidate = base;

  for (let suffix = 2; suffix < 1000; suffix += 1) {
    const existing = await withDatabaseRetry(() =>
      prisma.shop.findUnique({
        where: { slug: candidate },
        select: { id: true },
      }),
    );

    if (!existing) {
      return candidate;
    }

    candidate = `${base}-${suffix}`;
  }

  throw new Error('DATABASE_ERROR');
}


function buildShopImages(images: string[]) {
  return images.map((imageUrl, index) => ({
    imageUrl,
    sortOrder: index,
  }));
}

function buildShopCourses(courses: Shop['courses']) {
  return courses.map((course, index) => ({
    name: course.name.trim(),
    durationMinutes: parseInteger(course.duration),
    price: parseInteger(course.price),
    description: course.description?.trim() || null,
    sortOrder: index,
  }));
}

function buildShopSearchText(input: Pick<Shop, 'name' | 'regionLabel' | 'subRegionLabel' | 'themeLabel' | 'tagline' | 'description' | 'tags'>) {
  return [
    input.name.trim(),
    input.regionLabel.trim(),
    input.subRegionLabel?.trim() ?? '',
    input.themeLabel.trim(),
    input.tagline.trim(),
    input.description.trim(),
    ...input.tags.map((tag) => tag.trim()).filter(Boolean),
  ]
    .filter(Boolean)
    .join(' ');
}

function pushRegionFilter(and: Prisma.ShopWhereInput[], region?: string) {
  const regionVariants = getRegionVariants(region);
  if (regionVariants.length === 1) {
    and.push({ region: regionVariants[0] });
  } else if (regionVariants.length > 1) {
    and.push({ region: { in: regionVariants } });
  }
}

function buildShopPayload(input: Shop, slug = input.slug) {
  return {
    name: input.name.trim(),
    slug: slug.trim(),
    region: input.region,
    regionLabel: input.regionLabel,
    subRegion: input.subRegion?.trim() ? input.subRegion.trim() : null,
    subRegionLabel: input.subRegionLabel?.trim() ? input.subRegionLabel.trim() : null,
    theme: input.theme,
    themeLabel: input.themeLabel,
    tagline: input.tagline.trim(),
    description: input.description.trim(),
    address: input.address.trim(),
    phone: input.phone.trim(),
    hours: input.hours.trim(),
    isVisible: input.isVisible,
    isPremium: input.isPremium,
    premiumOrder: input.isPremium ? input.premiumOrder ?? 1 : null,
    thumbnailUrl: input.thumbnailUrl.trim() || null,
    bannerUrl: input.bannerUrl.trim() || null,
    ownerId: input.ownerId?.trim() ? input.ownerId.trim() : null,
    searchText: buildShopSearchText(input),
    tags: input.tags,
  };
}

function buildOwnerShopPayload(input: Shop) {
  const { ownerId, isPremium, premiumOrder, isVisible, ...payload } = buildShopPayload(input);
  void ownerId;
  void isPremium;
  void premiumOrder;
  void isVisible;
  return payload;
}

async function loadManagedShops(
  user: { id: string; role: UserRole; managedShopId?: string },
  filters: { region?: string; q?: string } = {},
) {
  const and: Prisma.ShopWhereInput[] = [];

  if (user.role === 'OWNER') {
    and.push({ OR: [{ ownerId: user.id }, ...(user.managedShopId ? [{ id: user.managedShopId }] : [])] });
  }

  pushRegionFilter(and, filters.region);

  if (filters.q) {
    and.push({ OR: [{ name: buildContainsFilter(filters.q) }, { phone: buildContainsFilter(filters.q) }] });
  }

  const where: Prisma.ShopWhereInput = and.length > 0 ? { AND: and } : {};

  const shops = await withDatabaseRetry(() =>
    prisma.shop.findMany({
      where,
      select: managedShopListSelect,
      orderBy: [{ isPremium: 'desc' }, { premiumOrder: 'asc' }, { name: 'asc' }],
    }),
  );

  return shops.map(mapManagedShopRecordForAdmin);
}

export async function listManagedShopsPage(
  user: { id: string; role: UserRole; managedShopId?: string },
  filters: { region?: string; q?: string; page?: number; pageSize?: number } = {},
) {
  const page = Math.max(1, Math.floor(filters.page ?? 1));
  const pageSize = Math.min(100, Math.max(10, Math.floor(filters.pageSize ?? 20)));
  const region = filters.region?.trim();
  const q = filters.q?.trim();
  const cacheKey = JSON.stringify({
    userId: user.id,
    role: user.role,
    managedShopId: user.managedShopId ?? '',
    region: region || '',
    q: q || '',
    page,
    pageSize,
  });

  return getInFlight(managedShopPageInFlight, cacheKey, async () => {
    const and: Prisma.ShopWhereInput[] = [];
    if (user.role === 'OWNER') {
      and.push({ OR: [{ ownerId: user.id }, ...(user.managedShopId ? [{ id: user.managedShopId }] : [])] });
    }
    pushRegionFilter(and, region);
    if (q) {
      and.push({ OR: [{ name: buildContainsFilter(q) }, { phone: buildContainsFilter(q) }] });
    }
    const where: Prisma.ShopWhereInput = and.length > 0 ? { AND: and } : {};

    try {
      const [shops, total] = await withDatabaseRetry(() =>
        Promise.all([
          prisma.shop.findMany({
            where,
            select: managedShopListSelect,
            orderBy: [{ isPremium: 'desc' }, { premiumOrder: 'asc' }, { name: 'asc' }],
            skip: (page - 1) * pageSize,
            take: pageSize,
          }),
          prisma.shop.count({ where }),
        ]),
      );

      return {
        shops: shops.map(mapManagedShopRecordForAdmin),
        total,
        page,
        pageSize,
        totalPages: Math.max(1, Math.ceil(total / pageSize)),
      };
    } catch (error) {
      console.error('Failed to list managed shops page:', error);
      throw new Error('DATABASE_ERROR');
    }
  });
}

const getCachedManagedShops = unstable_cache(
  async (
    userId: string,
    role: UserRole,
    managedShopId?: string,
    region?: string,
    q?: string,
  ) => loadManagedShops({ id: userId, role, managedShopId }, { region, q }),
  [MANAGED_SHOPS_CACHE_TAG, 'list'],
  {
    revalidate: 30,
    tags: [MANAGED_SHOPS_CACHE_TAG],
  },
);

async function loadManagedShopOptions(
  user: { id: string; role: UserRole; managedShopId?: string },
  filters: { region?: string; q?: string } = {},
) {
  const and: Prisma.ShopWhereInput[] = [];

  if (user.role === 'OWNER') {
    and.push({ OR: [{ ownerId: user.id }, ...(user.managedShopId ? [{ id: user.managedShopId }] : [])] });
  }

  pushRegionFilter(and, filters.region);

  if (filters.q) {
    and.push({ OR: [{ name: buildContainsFilter(filters.q) }, { phone: buildContainsFilter(filters.q) }] });
  }

  const where: Prisma.ShopWhereInput = and.length > 0 ? { AND: and } : {};
  const shops = await withDatabaseRetry(() =>
    prisma.shop.findMany({
      where,
      select: managedShopOptionSelect,
      orderBy: [{ isPremium: 'desc' }, { premiumOrder: 'asc' }, { name: 'asc' }],
    }),
  );

  return shops.map(mapManagedShopRecordToOption);
}

const getCachedManagedShopOptions = unstable_cache(
  async (
    userId: string,
    role: UserRole,
    managedShopId?: string,
    region?: string,
    q?: string,
  ) => loadManagedShopOptions({ id: userId, role, managedShopId }, { region, q }),
  [MANAGED_SHOPS_CACHE_TAG, 'options'],
  {
    revalidate: 30,
    tags: [MANAGED_SHOPS_CACHE_TAG],
  },
);

export async function listManagedShops(
  user: { id: string; role: UserRole; managedShopId?: string },
  filters: { region?: string; q?: string } = {},
) {
  const normalizedFilters = {
    region: filters.region?.trim() || undefined,
    q: filters.q?.trim() || undefined,
  };

  try {
    const cacheKey = JSON.stringify({ userId: user.id, role: user.role, managedShopId: user.managedShopId ?? '', ...normalizedFilters });
    return await getInFlight(managedShopsInFlight, cacheKey, async () => {
      try {
        return await getCachedManagedShops(
          user.id,
          user.role,
          user.managedShopId,
          normalizedFilters.region,
          normalizedFilters.q,
        );
      } catch (error) {
        if (!isMissingNextCacheContextError(error)) {
          throw error;
        }

        return await loadManagedShops(user, normalizedFilters);
      }
    });
  } catch (error) {
    console.error('Failed to list managed shops:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function listManagedShopOptions(
  user: { id: string; role: UserRole; managedShopId?: string },
  filters: { region?: string; q?: string } = {},
) {
  const normalizedFilters = {
    region: filters.region?.trim() || undefined,
    q: filters.q?.trim() || undefined,
  };

  try {
    const cacheKey = JSON.stringify({ userId: user.id, role: user.role, managedShopId: user.managedShopId ?? '', ...normalizedFilters });
    return await getInFlight(managedShopOptionsInFlight, cacheKey, async () => {
      try {
        return await getCachedManagedShopOptions(
          user.id,
          user.role,
          user.managedShopId,
          normalizedFilters.region,
          normalizedFilters.q,
        );
      } catch (error) {
        if (!isMissingNextCacheContextError(error)) {
          throw error;
        }

        return await loadManagedShopOptions(user, normalizedFilters);
      }
    });
  } catch (error) {
    console.error('Failed to list managed shop options:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function updatePremiumOrder(orderedIds: string[], visibilityById: Record<string, boolean> = {}) {
  const existingShops = await withDatabaseRetry(() =>
    prisma.shop.findMany({ select: { id: true, isVisible: true } }),
  );
  const existingShopMap = new Map(existingShops.map((shop) => [shop.id, shop]));
  const validIds = Array.from(new Set(orderedIds.filter((id) => existingShopMap.has(id))));

  await withDatabaseRetry(() =>
    prisma.$transaction([
      prisma.shop.updateMany({
        where: validIds.length > 0 ? { id: { notIn: validIds }, isPremium: true } : { isPremium: true },
        data: { isPremium: false, premiumOrder: null },
      }),
      ...validIds.map((id, index) =>
        prisma.shop.update({
          where: { id },
          data: {
            isPremium: true,
            premiumOrder: index + 1,
            isVisible: visibilityById[id] ?? existingShopMap.get(id)?.isVisible ?? true,
          },
        }),
      ),
    ]),
  );
  invalidatePublicShopCaches();
  invalidateManagedAdminCaches();

  return await getPremiumBoardData();
}

export async function getPremiumBoardData(): Promise<PremiumBoardData> {
  const shops = await listManagedShops({ id: 'admin', role: 'ADMIN' });

  return {
    premiumShops: shops.filter((shop) => shop.isPremium),
    availableShops: shops.filter((shop) => !shop.isPremium),
  };
}

type NoticeListOptions = {
  search?: string;
  strict?: boolean;
};

async function loadNotices(search?: string) {
  const notices = await withDatabaseRetry(() =>
    prisma.notice.findMany({
      where: search
        ? {
            OR: [{ title: buildContainsFilter(search) }, { content: buildContainsFilter(search) }],
          }
        : undefined,
      orderBy: [{ isPinned: 'desc' }, { createdAt: 'desc' }],
    }),
  );

  return notices.map(mapNotice);
}

const getCachedNotices = unstable_cache(loadNotices, [PUBLIC_BOARD_CACHE_TAG, 'notices'], {
  revalidate: 30,
  tags: [PUBLIC_BOARD_CACHE_TAG],
});

export async function listNotices(options: NoticeListOptions = {}) {
  const search = options.search?.trim() || undefined;

  try {
    return await getCachedNotices(search);
  } catch (error) {
    if (!isMissingNextCacheContextError(error)) {
      console.error('Failed to list notices:', error);
    }

    try {
      return await loadNotices(search);
    } catch (directError) {
      if (options.strict) {
        console.error('Failed to list notices:', directError);
        throw new Error('DATABASE_ERROR');
      }

      return [];
    }
  }
}

export async function getNoticeById(id: string) {
  const notice = await withDatabaseRetry(() => prisma.notice.findUnique({ where: { id } }));
  return notice ? mapNotice(notice) : null;
}

export async function createNotice(
  input: Pick<Notice, 'title' | 'content' | 'isPinned'> & { createdBy: string },
) {
  const notice = await withDatabaseRetry(() =>
    prisma.notice.create({
      data: {
        title: input.title.trim(),
        content: input.content.trim(),
        isPinned: input.isPinned,
        createdBy: input.createdBy,
      },
    }),
  );

  invalidatePublicBoardCaches();

  return mapNotice(notice);
}

export async function updateNotice(id: string, input: Pick<Notice, 'title' | 'content' | 'isPinned'>) {
  const existingNotice = await withDatabaseRetry(() =>
    prisma.notice.findUnique({
      where: { id },
      select: { id: true },
    }),
  );

  if (!existingNotice) {
    return null;
  }

  const notice = await withDatabaseRetry(() =>
    prisma.notice.update({
      where: { id },
      data: {
        title: input.title.trim(),
        content: input.content.trim(),
        isPinned: input.isPinned,
      },
    }),
  );

  invalidatePublicBoardCaches();

  return mapNotice(notice);
}

export async function deleteNotice(id: string) {
  const result = await withDatabaseRetry(() => prisma.notice.deleteMany({ where: { id } }));
  if (result.count > 0) {
    invalidatePublicBoardCaches();
  }
  return result.count > 0;
}

const DEFAULT_MANAGED_LIST_PAGE_SIZE = 100;
const MAX_MANAGED_LIST_PAGE_SIZE = 100;

function normalizeManagedPagination(options: { page?: number; pageSize?: number } = {}) {
  const page = Number.isInteger(options.page) && options.page && options.page > 0 ? options.page : 1;
  const requestedPageSize = Number.isInteger(options.pageSize) && options.pageSize && options.pageSize > 0
    ? options.pageSize
    : DEFAULT_MANAGED_LIST_PAGE_SIZE;
  const pageSize = Math.min(requestedPageSize, MAX_MANAGED_LIST_PAGE_SIZE);

  return {
    skip: (page - 1) * pageSize,
    take: pageSize,
  };
}

type QnaListOptions = {
  shopId?: string;
  shopOwnerId?: string;
  search?: string;
  viewer?: ViewerContext;
  page?: number;
  pageSize?: number;
  publicVisibleOnly?: boolean;
};

type PaginatedListResult<T> = {
  items: T[];
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
};

type PublicPaginationOptions = {
  page?: number;
  pageSize?: number;
};

const qnaInclude = {
  shop: {
    select: {
      ownerId: true,
      name: true,
      regionLabel: true,
    },
  },
  comments: {
    orderBy: {
      createdAt: 'asc',
    },
  },
} satisfies Prisma.QnAInclude;

const boardLandingQnaSelect = {
  id: true,
  userId: true,
  shopId: true,
  question: true,
  authorName: true,
  status: true,
  createdAt: true,
  shop: {
    select: {
      ownerId: true,
      name: true,
      regionLabel: true,
    },
  },
  comments: {
    select: {
      content: true,
      createdAt: true,
    },
    orderBy: {
      createdAt: 'desc',
    },
    take: 1,
  },
  _count: {
    select: {
      comments: true,
    },
  },
} satisfies Prisma.QnASelect;

function buildQnaWhere(shopId?: string, search?: string, shopOwnerId?: string, publicVisibleOnly = false) {
  return {
    ...(shopId ? { shopId } : {}),
    ...(shopOwnerId ? { shop: { ownerId: shopOwnerId } } : {}),
    ...(publicVisibleOnly ? { AND: [{ OR: [{ shopId: null }, { shop: { isVisible: true } }] }] } : {}),
    ...(search
      ? {
          OR: [
            { question: buildContainsFilter(search) },
            { authorName: buildContainsFilter(search) },
            { comments: { some: { content: buildContainsFilter(search) } } },
          ],
        }
      : {}),
  } satisfies Prisma.QnAWhereInput;
}

function buildLegacyQnaWhere(shopId?: string, search?: string, shopOwnerId?: string, publicVisibleOnly = false) {
  return {
    ...(shopId ? { shopId } : {}),
    ...(shopOwnerId ? { shop: { ownerId: shopOwnerId } } : {}),
    ...(publicVisibleOnly ? { AND: [{ OR: [{ shopId: null }, { shop: { isVisible: true } }] }] } : {}),
    ...(search
      ? {
          OR: [
            { question: buildContainsFilter(search) },
            { authorName: buildContainsFilter(search) },
          ],
        }
      : {}),
  } satisfies Prisma.QnAWhereInput;
}

function normalizePagination(options: PublicPaginationOptions) {
  const DEFAULT_PUBLIC_PAGE_SIZE = 10;
  const MAX_PUBLIC_PAGE_SIZE = 100;
  const rawPageSize =
    Number.isFinite(options.pageSize) && (options.pageSize ?? 0) > 0
      ? Math.floor(options.pageSize ?? DEFAULT_PUBLIC_PAGE_SIZE)
      : DEFAULT_PUBLIC_PAGE_SIZE;
  const pageSize = Math.min(rawPageSize, MAX_PUBLIC_PAGE_SIZE);
  const requestedPage = Number.isFinite(options.page) && (options.page ?? 0) > 0 ? Math.floor(options.page ?? 1) : 1;
  return { pageSize, requestedPage };
}

function buildReviewWhere(
  shopId?: string,
  search?: string,
  region?: string,
  searchType: 'all' | 'shop' | 'author' | 'content' = 'all',
) {
  const shopWhere = {
    isVisible: true,
    ...(region ? { region } : {}),
  } satisfies Prisma.ShopWhereInput;

  const searchWhere =
    searchType === 'shop'
      ? { shop: { ...shopWhere, name: buildContainsFilter(search ?? '') } }
      : searchType === 'author'
        ? { authorName: buildContainsFilter(search ?? '') }
        : searchType === 'content'
          ? { content: buildContainsFilter(search ?? '') }
          : {
              OR: [
                { content: buildContainsFilter(search ?? '') },
                { authorName: buildContainsFilter(search ?? '') },
                { shop: { name: buildContainsFilter(search ?? '') } },
              ],
            };

  return {
    isHidden: false,
    shop: shopWhere,
    ...(shopId ? { shopId } : {}),
    ...(search ? searchWhere : {}),
  } satisfies Prisma.ReviewWhereInput;
}

export async function listPublicQnaPage(
  options: QnaListOptions & PublicPaginationOptions = {},
): Promise<PaginatedListResult<QnA>> {
  const shopId = options.shopId?.trim();
  const search = options.search?.trim();
  const { pageSize, requestedPage } = normalizePagination(options);

  try {
    const where = buildQnaWhere(shopId, search, undefined, true);
    const totalItems = await withDatabaseRetry(() => prisma.qnA.count({ where }));
    const totalPages = getTotalPages(totalItems, pageSize);
    const page = clampPage(requestedPage, totalPages);
    const items = await withDatabaseRetry(() =>
      prisma.qnA.findMany({
        where,
        include: qnaInclude,
        orderBy: { createdAt: 'desc' },
        skip: (page - 1) * pageSize,
        take: pageSize,
      }),
    );

    return {
      items: items.map((entry) => {
        const mapped = mapQna(entry, options.viewer);
        if (mapped.canManage) {
          return mapped;
        }

        const rest = { ...mapped };
        delete rest.canManage;
        return rest;
      }),
      page,
      pageSize,
      totalItems,
      totalPages,
    };
  } catch (error) {
    if (!isQnaCommentStorageUnavailable(error)) {
      throw error;
    }

    const where = buildLegacyQnaWhere(shopId, search, undefined, true);
    const totalItems = await withDatabaseRetry(() => prisma.qnA.count({ where }));
    const totalPages = getTotalPages(totalItems, pageSize);
    const page = clampPage(requestedPage, totalPages);
    const items = await withDatabaseRetry(() =>
      loadLegacyQnaRecords(where, {
        skip: (page - 1) * pageSize,
        take: pageSize,
      }),
    );

    return {
      items: items.map((entry) => {
        const mapped = mapQna(entry, options.viewer);
        if (mapped.canManage) {
          return mapped;
        }

        const rest = { ...mapped };
        delete rest.canManage;
        return rest;
      }),
      page,
      pageSize,
      totalItems,
      totalPages,
    };
  }
}

async function loadQnaEntries(options: QnaListOptions = {}) {
  const shopId = options.shopId?.trim();
  const shopOwnerId = options.shopOwnerId?.trim();
  const search = options.search?.trim();
  const where = buildQnaWhere(shopId, search, shopOwnerId, options.publicVisibleOnly);

  try {
    const entries = await withDatabaseRetry(() =>
      prisma.qnA.findMany({
        where,
        include: qnaInclude,
        orderBy: { createdAt: 'desc' },
        ...normalizeManagedPagination(options),
      }),
    );

    return entries.map((entry) => mapQna(entry, options.viewer));
  } catch (error) {
    if (!isQnaCommentStorageUnavailable(error)) {
      throw error;
    }

    const legacyWhere = buildLegacyQnaWhere(shopId, search, shopOwnerId, options.publicVisibleOnly);
    const entries = await withDatabaseRetry(() =>
      loadLegacyQnaRecords(legacyWhere, normalizeManagedPagination(options)),
    );
    return entries.map((entry) => mapQna(entry, options.viewer));
  }
}

function shouldUseManagedQnaCache(options: QnaListOptions) {
  return Boolean(
    options.viewer &&
      !options.publicVisibleOnly &&
      (options.viewer.role === 'ADMIN' || options.viewer.role === 'OWNER'),
  );
}

const getCachedManagedQna = unstable_cache(
  async (
    shopId: string | undefined,
    shopOwnerId: string | undefined,
    search: string | undefined,
    viewerId: string,
    viewerRole: UserRole,
    page?: number,
    pageSize?: number,
  ) =>
    loadQnaEntries({
      shopId,
      shopOwnerId,
      search,
      viewer: { id: viewerId, role: viewerRole },
      page,
      pageSize,
    }),
  [MANAGED_QNA_CACHE_TAG, 'list'],
  {
    revalidate: 15,
    tags: [MANAGED_QNA_CACHE_TAG],
  },
);

export async function listQna(options: string | QnaListOptions = {}) {
  const normalizedOptions = typeof options === 'string' ? { shopId: options } : options;

  if (shouldUseManagedQnaCache(normalizedOptions)) {
    const viewer = normalizedOptions.viewer as ViewerContext;
    const shopId = normalizedOptions.shopId?.trim() || undefined;
    const shopOwnerId = normalizedOptions.shopOwnerId?.trim() || undefined;
    const search = normalizedOptions.search?.trim() || undefined;
    const cacheKey = JSON.stringify({
      shopId: shopId ?? '',
      shopOwnerId: shopOwnerId ?? '',
      search: search ?? '',
      viewerId: viewer.id,
      viewerRole: viewer.role,
      page: normalizedOptions.page ?? '',
      pageSize: normalizedOptions.pageSize ?? '',
    });

    try {
      return await getInFlight(managedQnaInFlight, cacheKey, async () =>
        getCachedManagedQna(
          shopId,
          shopOwnerId,
          search,
          viewer.id,
          viewer.role,
          normalizedOptions.page,
          normalizedOptions.pageSize,
        ),
      );
    } catch (error) {
      if (!isMissingNextCacheContextError(error)) {
        throw error;
      }
    }
  }

  return await loadQnaEntries(normalizedOptions);
}

type BoardLandingOptions = {
  includeReviews?: boolean;
  viewer?: ViewerContext;
};

export async function getBoardLandingData(options: BoardLandingOptions = {}) {
  const includeReviews = options.includeReviews ?? false;

  const [summary, notices, qnaEntries, reviews] = await Promise.all([
    getBoardSummary(),
    listNotices(),
    (async () => {
      try {
        const entries = await withDatabaseRetry(() =>
          prisma.qnA.findMany({
            select: boardLandingQnaSelect,
            where: { OR: [{ shopId: null }, { shop: { isVisible: true } }] },
            orderBy: { createdAt: 'desc' },
            take: 3,
          }),
        );

        return entries.map((entry) => mapBoardLandingQna(entry, options.viewer));
      } catch (error) {
        if (!isQnaCommentStorageUnavailable(error)) {
          throw error;
        }

        const entries = await withDatabaseRetry(() =>
          prisma.qnA.findMany({
            select: {
              id: true,
              userId: true,
              shopId: true,
              question: true,
              authorName: true,
              status: true,
              createdAt: true,
              shop: {
                select: {
                  ownerId: true,
                  name: true,
                  regionLabel: true,
                },
              },
            },
            where: { OR: [{ shopId: null }, { shop: { isVisible: true } }] },
            orderBy: { createdAt: 'desc' },
            take: 3,
          }),
        );

        return entries.map((entry) => mapBoardLandingQna(entry, options.viewer));
      }
    })(),
    includeReviews ? listReviews(3) : Promise.resolve([]),
  ]);

  return {
    summary,
    notices,
    qnaEntries,
    reviews,
  };
}

function canManageQnaEntry(entry: Prisma.QnAGetPayload<{ include: typeof qnaInclude }>, viewer?: ViewerContext) {
  if (!viewer || viewer.role === 'ADMIN') {
    return true;
  }

  return viewer.role === 'OWNER' && entry.shop?.ownerId === viewer.id;
}

export async function createQnaComment(
  id: string,
  input: {
    content: string;
    authorName: string;
    role: QnaCommentRole;
    userId?: string;
  },
  viewer?: ViewerContext,
) {
  try {
    const entry = await withDatabaseRetry(() =>
      prisma.$transaction(async (tx) => {
        const existingEntry = await tx.qnA.findUnique({
          where: { id },
          include: qnaInclude,
        });

        if (!existingEntry || !canManageQnaEntry(existingEntry, viewer)) {
          return null;
        }

        await tx.qnAComment.create({
          data: {
            qnaId: id,
            userId: input.userId ?? null,
            authorName: input.authorName.trim(),
            role: input.role,
            content: input.content.trim(),
          },
        });

        await tx.qnA.update({
          where: { id },
          data: {
            status: QnaStatus.ANSWERED,
          },
        });

        return await tx.qnA.findUnique({
          where: { id },
          include: qnaInclude,
        });
      }),
    );

    return entry ? mapQna(entry, viewer) : null;
  } catch (error) {
    if (isQnaCommentStorageUnavailable(error)) {
      throw new Error('Q&A 댓글 기능 배포가 아직 완료되지 않았습니다. 데이터베이스 마이그레이션을 먼저 적용해 주세요.');
    }

    throw error;
  }
}

export async function answerQna(
  id: string,
  answer: string,
  answeredBy?: string,
  answeredByName = '운영자',
  role: QnaCommentRole = QnaCommentRole.ADMIN,
  viewer?: ViewerContext,
) {
  try {
    const entry = await withDatabaseRetry(() =>
      prisma.$transaction(async (tx) => {
        const existingEntry = await tx.qnA.findUnique({
          where: { id },
          include: qnaInclude,
        });

        if (!existingEntry || !canManageQnaEntry(existingEntry, viewer)) {
          return null;
        }

        if (existingEntry.status === QnaStatus.ANSWERED) {
          return existingEntry;
        }

        const claimed = await tx.qnA.updateMany({
          where: { id, status: QnaStatus.OPEN },
          data: { status: QnaStatus.ANSWERED },
        });

        if (claimed.count === 0) {
          return await tx.qnA.findUnique({
            where: { id },
            include: qnaInclude,
          });
        }

        await tx.qnAComment.create({
          data: {
            qnaId: id,
            userId: answeredBy ?? null,
            authorName: answeredByName.trim(),
            role,
            content: answer.trim(),
          },
        });

        return await tx.qnA.findUnique({
          where: { id },
          include: qnaInclude,
        });
      }),
    );

    return entry ? mapQna(entry, viewer) : null;
  } catch (error) {
    if (isQnaCommentStorageUnavailable(error)) {
      throw new Error('Q&A 댓글 기능 배포가 아직 완료되지 않았습니다. 데이터베이스 마이그레이션을 먼저 적용해 주세요.');
    }

    throw error;
  }
}

export async function createQna(
  input: Pick<QnA, 'question' | 'authorName'> & { shopId?: string; userId?: string },
  viewer?: ViewerContext,
) {
  try {
    const entry = await withDatabaseRetry(() =>
      prisma.qnA.create({
        data: {
          question: input.question.trim(),
          authorName: input.authorName.trim(),
          shopId: input.shopId?.trim() || null,
          userId: input.userId?.trim() || null,
          status: QnaStatus.OPEN,
        },
        include: qnaInclude,
      }),
    );

    invalidatePublicBoardCaches();

    return mapQna(entry, viewer);
  } catch (error) {
    if (!isQnaCommentStorageUnavailable(error)) {
      throw error;
    }

    const entry = await withDatabaseRetry(() =>
      prisma.qnA.create({
        data: {
          question: input.question.trim(),
          authorName: input.authorName.trim(),
          shopId: input.shopId?.trim() || null,
          userId: input.userId?.trim() || null,
          status: QnaStatus.OPEN,
        },
        include: {
          shop: {
            select: {
              ownerId: true,
              name: true,
              regionLabel: true,
            },
          },
        },
      }),
    );

    invalidatePublicBoardCaches();

    return mapQna({ ...entry, comments: [] }, viewer);
  }
}

type ReviewSearchType = 'all' | 'shop' | 'author' | 'content';

type ReviewListOptions = {
  limit?: number;
  shopId?: string;
  region?: string;
  search?: string;
  searchType?: ReviewSearchType;
  page?: number;
  pageSize?: number;
  viewer?: ViewerContext;
};

const PUBLIC_REVIEWER_EMAIL = 'public-reviewer@massage.local';
const PUBLIC_REVIEWER_PASSWORD_HASH = 'public-reviewer';

async function getPublicReviewerId() {
  const reviewer = await withDatabaseRetry(() =>
    prisma.user.upsert({
      where: { email: PUBLIC_REVIEWER_EMAIL },
      update: {
        name: '공개 리뷰 작성자',
        role: PrismaUserRole.USER,
        status: UserStatus.APPROVED,
        passwordHash: PUBLIC_REVIEWER_PASSWORD_HASH,
      },
      create: {
        email: PUBLIC_REVIEWER_EMAIL,
        name: '공개 리뷰 작성자',
        role: PrismaUserRole.USER,
        status: UserStatus.APPROVED,
        passwordHash: PUBLIC_REVIEWER_PASSWORD_HASH,
      },
      select: { id: true },
    }),
  );

  return reviewer.id;
}

async function refreshShopReviewRating(
  shopId: string,
  client: typeof prisma | Prisma.TransactionClient = prisma,
) {
  const aggregate = await client.review.aggregate({
    where: { shopId, isHidden: false },
    _avg: { rating: true },
    _count: { _all: true },
  });

  await client.shop.update({
    where: { id: shopId },
    data: {
      rating: aggregate._avg.rating ?? 0,
      reviewCount: aggregate._count._all,
    },
  });
}

export async function createReview(input: {
  shopId: string;
  userId?: string;
  authorName: string;
  rating: number;
  content: string;
}) {
  const userId = input.userId?.trim() || (await getPublicReviewerId());

  const review = await withDatabaseRetry(() =>
    prisma.review.create({
      data: {
        shopId: input.shopId,
        userId,
        authorName: input.authorName.trim(),
        rating: input.rating,
        content: input.content.trim(),
      },
      include: { shop: { select: { name: true, slug: true } } },
    }),
  );

  await withDatabaseRetry(() => refreshShopReviewRating(input.shopId));
  invalidatePublicShopCaches();
  invalidatePublicBoardCaches();
  try {
    if (review.shop?.slug) {
      revalidatePath(`/shop/${review.shop.slug}`);
    }
  } catch {
    // best-effort detail route revalidation; ignored outside a request scope
  }
  return mapReview(review);
}

function mapPublicReview(review: Review, viewer?: ViewerContext): Review {
  const canManage = Boolean(viewer && (viewer.role === 'ADMIN' || viewer.id === review.userId));
  const publicReview = { ...review };
  delete publicReview.userId;
  return canManage ? { ...publicReview, canManage } : publicReview;
}

export async function listPublicReviewPage(
  options: ReviewListOptions & PublicPaginationOptions = {},
): Promise<PaginatedListResult<Review>> {
  const shopId = options.shopId?.trim();
  const region = options.region?.trim();
  const search = options.search?.trim();
  const searchType = options.searchType ?? 'all';
  const { pageSize, requestedPage } = normalizePagination(options);
  const where = buildReviewWhere(shopId, search, region, searchType);
  const totalItems = await withDatabaseRetry(() => prisma.review.count({ where }));
  const totalPages = getTotalPages(totalItems, pageSize);
  const page = clampPage(requestedPage, totalPages);
  const items = await withDatabaseRetry(() =>
    prisma.review.findMany({
      where,
      include: { shop: { select: { name: true } } },
      orderBy: { createdAt: 'desc' },
      skip: (page - 1) * pageSize,
      take: pageSize,
    }),
  );

  return {
    items: items.map((review) => mapPublicReview(mapReview(review), options.viewer)),
    page,
    pageSize,
    totalItems,
    totalPages,
  };
}

async function loadReviews(normalizedOptions: ReviewListOptions) {
  const shopId = normalizedOptions.shopId?.trim();
  const region = normalizedOptions.region?.trim();
  const search = normalizedOptions.search?.trim();
  const searchType = normalizedOptions.searchType ?? 'all';

  const reviews = await withDatabaseRetry(() =>
    prisma.review.findMany({
      where: buildReviewWhere(shopId, search, region, searchType),
      include: { shop: { select: { name: true } } },
      orderBy: { createdAt: 'desc' },
      ...(typeof normalizedOptions.limit === 'number' ? { take: normalizedOptions.limit } : {}),
    }),
  );

  return reviews.map((review) => mapPublicReview(mapReview(review), normalizedOptions.viewer));
}

const getCachedReviews = unstable_cache(loadReviews, [PUBLIC_BOARD_CACHE_TAG, 'reviews'], {
  revalidate: 30,
  tags: [PUBLIC_BOARD_CACHE_TAG],
});

export async function listReviews(options: number | ReviewListOptions = {}) {
  const normalizedOptions = typeof options === 'number' ? { limit: options } : options;

  try {
    return await getCachedReviews(normalizedOptions);
  } catch (error) {
    if (!isMissingNextCacheContextError(error)) {
      console.error('Failed to load reviews:', error);
    }

    try {
      return await loadReviews(normalizedOptions);
    } catch {
      return [];
    }
  }
}

export async function listManagedReviews(user: { id: string; role: UserRole }, search?: string, pagination: { page?: number; pageSize?: number } = {}) {
  const normalizedSearch = search?.trim();
  const cacheKey = JSON.stringify({
    userId: user.id,
    role: user.role,
    search: normalizedSearch ?? '',
    page: pagination.page ?? '',
    pageSize: pagination.pageSize ?? '',
  });

  return getInFlight(managedReviewsInFlight, cacheKey, async () => {
    const reviewWhere =
      user.role === 'ADMIN'
        ? {
            ...(normalizedSearch
              ? {
                  OR: [
                    { content: buildContainsFilter(normalizedSearch) },
                    { authorName: buildContainsFilter(normalizedSearch) },
                    { shop: { name: buildContainsFilter(normalizedSearch) } },
                  ],
                }
              : {}),
          }
        : {
            shop: {
              ownerId: user.id,
            },
            ...(normalizedSearch
              ? {
                  OR: [
                    { content: buildContainsFilter(normalizedSearch) },
                    { authorName: buildContainsFilter(normalizedSearch) },
                    { shop: { name: buildContainsFilter(normalizedSearch) } },
                  ],
                }
              : {}),
          };

    const reviews = await withDatabaseRetry(() =>
      prisma.review.findMany({
        where: reviewWhere,
        include: { shop: { select: { name: true, regionLabel: true } } },
        orderBy: { createdAt: 'desc' },
        ...normalizeManagedPagination(pagination),
      }),
    );

    return reviews.map((review) => ({
      ...mapReview(review),
      shopRegionLabel: review.shop.regionLabel,
    }));
  });
}

export async function updateReview(
  id: string,
  input: { rating?: number; content?: string; authorName?: string },
) {
  const trimmedContent = input.content?.trim();
  const trimmedAuthorName = input.authorName?.trim();

  if (input.content !== undefined && !trimmedContent) {
    return null;
  }

  if (input.authorName !== undefined && !trimmedAuthorName) {
    return null;
  }

  try {
    const updated = await withDatabaseRetry(() =>
      prisma.$transaction(async (tx) => {
        const review = await tx.review.update({
          where: { id },
          data: {
            ...(typeof input.rating === 'number' ? { rating: input.rating } : {}),
            ...(trimmedContent !== undefined ? { content: trimmedContent } : {}),
            ...(trimmedAuthorName !== undefined ? { authorName: trimmedAuthorName } : {}),
          },
          include: { shop: { select: { name: true } } },
        });

        if (input.rating !== undefined) {
          await refreshShopReviewRating(review.shopId, tx);
        }

        return review;
      }),
    );

    invalidatePublicShopCaches();
    invalidatePublicBoardCaches();
    return mapReview(updated);
  } catch (error) {
    if (error instanceof Prisma.PrismaClientKnownRequestError && error.code === 'P2025') {
      return null;
    }

    throw error;
  }
}

export async function deleteReview(reviewId: string) {
  try {
    await withDatabaseRetry(() =>
      prisma.$transaction(async (tx) => {
        const review = await tx.review.delete({
          where: { id: reviewId },
          select: { shopId: true },
        });

        await refreshShopReviewRating(review.shopId, tx);
      }),
    );

    invalidatePublicShopCaches();
    invalidatePublicBoardCaches();
    return true;
  } catch (error) {
    if (error instanceof Prisma.PrismaClientKnownRequestError && error.code === 'P2025') {
      return false;
    }

    throw error;
  }
}

export async function updateQna(
  id: string,
  input: { question: string },
  viewer?: ViewerContext,
) {
  try {
    const question = input.question.trim();
    if (!question) {
      return null;
    }

    const entry = await withDatabaseRetry(() =>
      prisma.qnA.update({
        where: { id },
        data: {
          question,
        },
        include: qnaInclude,
      }),
    );
    invalidatePublicBoardCaches();
    return mapQna(entry, viewer);
  } catch (error) {
    if (isRecordNotFoundError(error)) {
      return null;
    }
    console.error('Failed to update qna:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function updatePublicQna(
  id: string,
  userId: string,
  input: { question: string },
  viewer?: ViewerContext,
) {
  try {
    const question = input.question.trim();
    if (!question) {
      return null;
    }

    const updated = await withDatabaseRetry(() =>
      prisma.qnA.updateMany({
        where: {
          id,
          userId,
          status: QnaStatus.OPEN,
          comments: { none: {} },
        },
        data: { question },
      }),
    );

    if (updated.count === 0) {
      return null;
    }

    const entry = await withDatabaseRetry(() =>
      prisma.qnA.findUnique({
        where: { id },
        include: qnaInclude,
      }),
    );

    invalidatePublicBoardCaches();
    return entry ? mapQna(entry, viewer) : null;
  } catch (error) {
    if (isRecordNotFoundError(error)) {
      return null;
    }
    console.error('Failed to update public qna:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function deleteQna(id: string) {
  try {
    await withDatabaseRetry(() => prisma.qnA.delete({ where: { id } }));
    invalidatePublicBoardCaches();
    return true;
  } catch (error) {
    if (isRecordNotFoundError(error)) {
      return false;
    }
    console.error('Failed to delete qna:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function deletePublicQna(id: string, userId: string) {
  try {
    const deleted = await withDatabaseRetry(() =>
      prisma.qnA.deleteMany({
        where: {
          id,
          userId,
          status: QnaStatus.OPEN,
          comments: { none: {} },
        },
      }),
    );

    if (deleted.count === 0) {
      return false;
    }

    invalidatePublicBoardCaches();
    return true;
  } catch (error) {
    if (isRecordNotFoundError(error)) {
      return false;
    }
    console.error('Failed to delete public qna:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function setReviewHiddenState(
  user: { id: string; role: UserRole },
  reviewId: string,
  isHidden: boolean,
) {
  try {
    const updated = await withDatabaseRetry(() =>
      prisma.$transaction(async (tx) => {
        const review = await tx.review.findUnique({
          where: { id: reviewId },
          include: {
            shop: {
              select: {
                ownerId: true,
              },
            },
          },
        });

        if (!review) {
          return null;
        }

        if (user.role !== 'ADMIN' && review.shop.ownerId !== user.id) {
          return null;
        }

        const nextReview = await tx.review.update({
          where: { id: reviewId },
          data: { isHidden },
          include: { shop: { select: { name: true } } },
        });

        await refreshShopReviewRating(review.shopId, tx);

        return nextReview;
      }),
    );

    if (!updated) {
      return null;
    }

    invalidatePublicShopCaches();
    invalidatePublicBoardCaches();

    return mapReview(updated);
  } catch (error) {
    if (error instanceof Prisma.PrismaClientKnownRequestError && error.code === 'P2025') {
      return null;
    }

    throw error;
  }
}

export async function deleteManagedReview(user: { id: string; role: UserRole }, reviewId: string) {
  try {
    const deleted = await withDatabaseRetry(() =>
      prisma.$transaction(async (tx) => {
        const review = await tx.review.findUnique({
          where: { id: reviewId },
          include: {
            shop: {
              select: {
                ownerId: true,
              },
            },
          },
        });

        if (!review) {
          return false;
        }

        if (user.role !== 'ADMIN' && review.shop.ownerId !== user.id) {
          return false;
        }

        await tx.review.delete({ where: { id: reviewId } });
        await refreshShopReviewRating(review.shopId, tx);
        return true;
      }),
    );

    if (!deleted) {
      return false;
    }

    invalidatePublicShopCaches();
    invalidatePublicBoardCaches();
    return true;
  } catch (error) {
    if (error instanceof Prisma.PrismaClientKnownRequestError && error.code === 'P2025') {
      return false;
    }

    throw error;
  }
}

export async function deleteManagedQna(user: { id: string; role: UserRole }, qnaId: string) {
  const qna = await withDatabaseRetry(() =>
    prisma.qnA.findUnique({
      where: { id: qnaId },
      select: {
        shop: {
          select: {
            ownerId: true,
          },
        },
      },
    }),
  );

  if (!qna) {
    return false;
  }

  if (user.role !== 'ADMIN' && qna.shop?.ownerId !== user.id) {
    return false;
  }

  try {
    await withDatabaseRetry(() => prisma.qnA.delete({ where: { id: qnaId } }));
  } catch (error) {
    if (error instanceof Prisma.PrismaClientKnownRequestError && error.code === 'P2025') {
      return false;
    }

    throw error;
  }

  invalidatePublicBoardCaches();
  return true;
}

export async function getAdminShopById(id: string) {
  const shop = await withDatabaseRetry(() =>
    prisma.shop.findUnique({
      where: { id },
      include: adminShopDetailInclude,
    }),
  );

  return shop ? mapShop({ ...shop, reviews: [] }) : null;
}

export async function getQnaShopOwnerId(id: string) {
  const qna = await withDatabaseRetry(() =>
    prisma.qnA.findUnique({
      where: { id },
      select: {
        shop: {
          select: {
            ownerId: true,
          },
        },
      },
    }),
  );

  if (!qna) {
    return { exists: false, ownerId: null };
  }

  return {
    exists: true,
    ownerId: qna.shop?.ownerId ?? null,
  };
}

export async function createAdminShop(input: Shop) {
  const slug = await createUniqueShopSlug(input);
  const shop = await withDatabaseRetry(() =>
    prisma.$transaction(async (tx) => {
      const createdShop = await tx.shop.create({
        data: {
          ...buildShopPayload(input, slug),
          images: {
            create: buildShopImages(input.images),
          },
          courses: {
            create: buildShopCourses(input.courses),
          },
        },
        include: shopInclude,
      });

      if (input.ownerId) {
        await tx.user.updateMany({
          where: { id: input.ownerId, managedShopId: null },
          data: { managedShopId: createdShop.id },
        });
      }

      return createdShop;
    }),
  );

  invalidatePublicShopCaches();
  invalidateManagedAdminCaches();

  return mapShop(shop);
}

export async function updateAdminShop(id: string, input: Shop, access?: { ownerId?: string }) {
  try {
    const shop = await withDatabaseRetry(() =>
      access?.ownerId
        ? prisma.$transaction(async (tx) => {
            const updated = await tx.shop.updateMany({
              where: { id, ownerId: access.ownerId },
              data: buildOwnerShopPayload(input),
            });

            if (updated.count === 0) {
              return null;
            }

            await tx.shopImage.deleteMany({ where: { shopId: id } });
            await tx.shopCourse.deleteMany({ where: { shopId: id } });

            if (input.images.length > 0) {
              await tx.shopImage.createMany({
                data: buildShopImages(input.images).map((image) => ({ ...image, shopId: id })),
              });
            }

            if (input.courses.length > 0) {
              await tx.shopCourse.createMany({
                data: buildShopCourses(input.courses).map((course) => ({ ...course, shopId: id })),
              });
            }

            return tx.shop.findUnique({
              where: { id },
              include: shopInclude,
            });
          })
        : prisma.shop.update({
            where: { id },
            data: {
              ...buildShopPayload(input),
              images: {
                deleteMany: {},
                create: buildShopImages(input.images),
              },
              courses: {
                deleteMany: {},
                create: buildShopCourses(input.courses),
              },
            },
            include: shopInclude,
          }),
    );

    if (!shop) {
      return null;
    }

    invalidatePublicShopCaches();
    invalidateManagedAdminCaches();

    return mapShop(shop);
  } catch (error) {
    if (error instanceof Prisma.PrismaClientKnownRequestError && error.code === 'P2025') {
      return null;
    }
    console.error('Failed to update admin shop:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function deleteAdminShop(id: string, access?: { ownerId?: string }) {
  try {
    const deleted = await withDatabaseRetry(() =>
      prisma.shop.deleteMany({
        where: access?.ownerId ? { id, ownerId: access.ownerId } : { id },
      }),
    );

    if (deleted.count === 0) {
      return false;
    }

    invalidatePublicShopCaches();
    invalidateManagedAdminCaches();

    return true;
  } catch (error) {
    console.error('Failed to delete admin shop:', error);
    throw new Error('DATABASE_ERROR');
  }
}

async function loadBoardSummary() {
  const [notices, qna, reviews] = await withDatabaseRetry(() =>
    Promise.all([
      prisma.notice.count(),
      prisma.qnA.count({ where: { OR: [{ shopId: null }, { shop: { isVisible: true } }] } }),
      prisma.review.count({ where: { isHidden: false, shop: { isVisible: true } } }),
    ]),
  );

  return { notices, qna, reviews };
}

const getCachedBoardSummary = unstable_cache(loadBoardSummary, [PUBLIC_BOARD_CACHE_TAG, 'summary'], {
  revalidate: 30,
  tags: [PUBLIC_BOARD_CACHE_TAG],
});

export async function getBoardSummary() {
  try {
    return await getCachedBoardSummary();
  } catch (error) {
    if (!isMissingNextCacheContextError(error)) {
      console.error('Failed to load board summary:', error);
    }

    try {
      return await loadBoardSummary();
    } catch {
      return { notices: 0, qna: 0, reviews: 0 };
    }
  }
}

export async function listPartnershipInquiries() {
  const entries = await withDatabaseRetry(() =>
    prisma.partnershipInquiry.findMany({
      orderBy: { createdAt: 'desc' },
    }),
  );

  return entries.map(mapPartnershipInquiry);
}

export async function createPartnershipInquiry(
  input: Omit<PartnershipInquiry, 'id' | 'createdAt' | 'status'> & { status?: PartnershipInquiry['status'] },
) {
  const entry = await withDatabaseRetry(() =>
    prisma.partnershipInquiry.create({
      data: {
        shopName: input.shopName.trim(),
        region: input.region.trim(),
        subRegion: input.subRegion.trim(),
        theme: input.theme.trim(),
        contactName: input.contactName.trim(),
        phone: input.phone.trim(),
        kakaoId: input.kakaoId?.trim() || null,
        message: input.message.trim(),
        status: mapPartnershipStatus(input.status ?? 'pending'),
      },
    }),
  );

  return mapPartnershipInquiry(entry);
}

export async function updatePartnershipInquiryStatus(
  id: string,
  status: PartnershipInquiry['status'],
) {
  try {
    const entry = await withDatabaseRetry(() =>
      prisma.partnershipInquiry.update({
        where: { id },
        data: { status: mapPartnershipStatus(status) },
      }),
    );

    return mapPartnershipInquiry(entry);
  } catch (error) {
    if (isRecordNotFoundError(error)) {
      return null;
    }
    console.error('Failed to update partnership inquiry status:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function deletePartnershipInquiry(id: string) {
  const result = await withDatabaseRetry(() =>
    prisma.partnershipInquiry.deleteMany({
      where: { id },
    }),
  );

  return result.count > 0;
}

export async function getSiteContent() {
  const loaded = await loadSiteContentRecord();

  if (!loaded) {
    return null;
  }

  const { record, content } = loaded;

  if (hasLegacySiteContent(record, content)) {
    await withDatabaseRetry(() =>
      prisma.siteSettings.update({
        where: { id: SITE_SETTINGS_ID },
        data: {
          siteName: content.siteSettings.siteName,
          siteTitle: content.siteSettings.siteTitle,
          siteDescription: content.siteSettings.siteDescription,
          heroMainText: content.siteSettings.heroMainText,
          heroSubText: content.siteSettings.heroSubText,
          contactPhone: content.siteSettings.contactPhone,
          footerInfo: content.siteSettings.footerInfo,
          seoSection1Title: content.homeSeo.section1Title,
          seoSection1Content: content.homeSeo.section1Content,
          seoSection2Title: content.homeSeo.section2Title,
          seoSection2Content: content.homeSeo.section2Content,
          seoSection3Title: content.homeSeo.section3Title,
          seoSection3Content: content.homeSeo.section3Content,
        },
      }),
    );
  }


  return content;
}

export async function getPublicSiteContent() {
  if (!cachedPublicSiteContentPromise) {
    cachedPublicSiteContentPromise = readPublicSiteContentWithFallback().finally(() => {
      cachedPublicSiteContentPromise = null;
    });
  }

  return cachedPublicSiteContentPromise;
}

export async function upsertSiteContent(input: SiteSettings & HomeSeoContent) {
  const sanitizedInput = sanitizeSiteContentInput(input);
  const record = await withDatabaseRetry(() =>
    prisma.siteSettings.upsert({
      where: { id: SITE_SETTINGS_ID },
      update: {
        siteName: sanitizedInput.siteName,
        siteTitle: sanitizedInput.siteTitle,
        siteDescription: sanitizedInput.siteDescription,
        heroMainText: sanitizedInput.heroMainText,
        heroSubText: sanitizedInput.heroSubText,
        contactPhone: sanitizedInput.contactPhone,
        footerInfo: sanitizedInput.footerInfo,
        seoSection1Title: sanitizedInput.section1Title,
        seoSection1Content: sanitizedInput.section1Content,
        seoSection2Title: sanitizedInput.section2Title,
        seoSection2Content: sanitizedInput.section2Content,
        seoSection3Title: sanitizedInput.section3Title,
        seoSection3Content: sanitizedInput.section3Content,
      },
      create: {
        id: SITE_SETTINGS_ID,
        siteName: sanitizedInput.siteName,
        siteTitle: sanitizedInput.siteTitle,
        siteDescription: sanitizedInput.siteDescription,
        heroMainText: sanitizedInput.heroMainText,
        heroSubText: sanitizedInput.heroSubText,
        contactPhone: sanitizedInput.contactPhone,
        footerInfo: sanitizedInput.footerInfo,
        seoSection1Title: sanitizedInput.section1Title,
        seoSection1Content: sanitizedInput.section1Content,
        seoSection2Title: sanitizedInput.section2Title,
        seoSection2Content: sanitizedInput.section2Content,
        seoSection3Title: sanitizedInput.section3Title,
        seoSection3Content: sanitizedInput.section3Content,
      },
    }),
  );

  const content = mapSiteSettings(record);
  cachedPublicSiteContentPromise = null;
  safeRevalidateTag(PUBLIC_SITE_CONTENT_CACHE_TAG);

  return content;
}
function sanitizeSiteContentInput(input: SiteSettings & HomeSeoContent) {
  return {
    siteName: input.siteName.trim(),
    siteTitle: input.siteTitle.trim(),
    siteDescription: input.siteDescription.trim(),
    heroMainText: input.heroMainText.trim(),
    heroSubText: input.heroSubText.trim(),
    contactPhone: input.contactPhone.trim(),
    footerInfo: input.footerInfo.trim(),
    section1Title: sanitizeBoundedText(input.section1Title, HOME_SEO_TITLE_MAX_LENGTH),
    section1Content: sanitizeBoundedText(input.section1Content, HOME_SEO_CONTENT_MAX_LENGTH),
    section2Title: sanitizeBoundedText(input.section2Title, HOME_SEO_TITLE_MAX_LENGTH),
    section2Content: sanitizeBoundedText(input.section2Content, HOME_SEO_CONTENT_MAX_LENGTH),
    section3Title: sanitizeBoundedText(input.section3Title, HOME_SEO_TITLE_MAX_LENGTH),
    section3Content: sanitizeBoundedText(input.section3Content, HOME_SEO_CONTENT_MAX_LENGTH),
  };
}

export async function getAdminDashboardData(): Promise<AdminDashboardData> {
  const [summaryRow, pendingQna, recentReviews] = await withDatabaseRetry(async () => {
    const [summaryRows, pendingQnaRows, recentReviewRows] = await Promise.all([
      prisma.$queryRaw<
        Array<{
          shopCount: bigint | number;
          premiumCount: bigint | number;
          unansweredCount: bigint | number;
          noticeCount: bigint | number;
        }>
      >(Prisma.sql`
        SELECT
          (SELECT COUNT(*)::bigint FROM "shops") AS "shopCount",
          (SELECT COUNT(*)::bigint FROM "shops" WHERE "is_premium" = true) AS "premiumCount",
          (SELECT COUNT(*)::bigint FROM "qna" WHERE "status" = ${QnaStatus.OPEN}) AS "unansweredCount",
          (SELECT COUNT(*)::bigint FROM "notices") AS "noticeCount"
      `),
      prisma.qnA.findMany({
        where: { status: QnaStatus.OPEN },
        orderBy: { createdAt: 'desc' },
        take: 4,
      }),
      prisma.review.findMany({
        include: { shop: { select: { name: true } } },
        orderBy: { createdAt: 'desc' },
        take: 4,
      }),
    ]);

    return [summaryRows[0], pendingQnaRows, recentReviewRows] as const;
  });

  const shopCount = Number(summaryRow?.shopCount ?? 0);
  const premiumCount = Number(summaryRow?.premiumCount ?? 0);
  const unansweredCount = Number(summaryRow?.unansweredCount ?? 0);
  const noticeCount = Number(summaryRow?.noticeCount ?? 0);

  return {
    summary: [
      { label: '전체 업소', value: shopCount },
      { label: '프리미엄 업소', value: premiumCount },
      { label: '미답변 Q&A', value: unansweredCount },
      { label: '공지 수', value: noticeCount },
    ],
    pendingQna: pendingQna.map((item) => ({
      id: item.id,
      question: item.question,
      isAnswered: false,
    })),
    recentReviews: recentReviews.map((item) => ({
      id: item.id,
      shopName: item.shop.name,
      rating: item.rating,
      content: item.content,
    })),
  };
}

export const getCachedAdminDashboardData = unstable_cache(getAdminDashboardData, ['admin-dashboard-data'], {
  revalidate: 60,
  tags: [ADMIN_DASHBOARD_CACHE_TAG],
});

export async function getAdminStatsData(): Promise<AdminStatsData> {
  const [shopCount, premiumCount, unansweredCount, visibleReviewCount, topShops] = await withDatabaseRetry(() =>
    Promise.all([
      prisma.shop.count(),
      prisma.shop.count({ where: { isPremium: true } }),
      prisma.qnA.count({ where: { status: QnaStatus.OPEN } }),
      prisma.review.count({ where: { isHidden: false } }),
      prisma.shop.findMany({
        select: {
          id: true,
          name: true,
          regionLabel: true,
          reviewCount: true,
        },
        orderBy: [{ reviewCount: 'desc' }, { name: 'asc' }],
        take: 5,
      }),
    ]),
  );

  return {
    summary: [
      { label: '전체 업소', value: shopCount, helperText: '등록된 전체 업소 수' },
      { label: '프리미엄 업소', value: premiumCount, helperText: '상단 노출 중인 프리미엄 업소' },
      { label: '미답변 Q&A', value: unansweredCount, helperText: '아직 답변이 필요한 문의' },
      { label: '노출 중 리뷰', value: visibleReviewCount, helperText: '숨김 처리되지 않은 공개 리뷰' },
    ],
    topShops: topShops.map((shop) => ({
      id: shop.id,
      name: shop.name,
      regionLabel: shop.regionLabel,
      viewCount: shop.reviewCount,
    })),
  };
}
