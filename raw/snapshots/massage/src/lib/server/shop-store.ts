import { revalidateTag, unstable_cache } from 'next/cache';
import { cache } from 'react';
import type { Review as DbReview, Shop as DbShop, ShopCourse, ShopImage } from '@prisma/client';
import { Prisma } from '@prisma/client';
import type { Review, Shop, ShopListItem } from '@/lib/types';
import { REGION_MAP } from '@/lib/catalog';
import { prisma } from '@/lib/db/prisma';
import { withDatabaseRetry, isRecordNotFoundError } from '@/lib/db/retry';
import { runWithConcurrencyLimit } from '@/lib/async/run-with-concurrency-limit';
import { getSharedInFlight } from '@/lib/server/in-flight';

import type { ShopMediaVariant } from '@/lib/server/shop-media';

function getShopMediaVersion(updatedAt: Date) {
  return updatedAt.getTime();
}

function buildShopThumbnailProxyUrl(slug: string, updatedAt: Date, size: ShopMediaVariant) {
  return `/api/shops/${encodeURIComponent(slug)}/thumbnail?v=${getShopMediaVersion(updatedAt)}&size=${size}`;
}

function buildShopBannerProxyUrl(slug: string, updatedAt: Date, size: ShopMediaVariant) {
  return `/api/shops/${encodeURIComponent(slug)}/banner?v=${getShopMediaVersion(updatedAt)}&size=${size}`;
}

function buildShopGalleryImageProxyUrl(slug: string, updatedAt: Date, index: number, size: ShopMediaVariant) {
  return `/api/shops/${encodeURIComponent(slug)}/images/${index}?v=${getShopMediaVersion(updatedAt)}&size=${size}`;
}

interface ShopFilters {
  region?: string;
  subRegion?: string;
  theme?: string;
  query?: string;
  sort?: string;
  regularOffset?: number;
  regularLimit?: number;
}

interface DirectoryShopFilters extends ShopFilters {
  includePremium?: boolean;
}

type ShopListResponse = {
  allShops: ShopListItem[];
  premiumShops: ShopListItem[];
  regularShops: ShopListItem[];
  regularTotal: number;
  total: number;
};

export type ShopRecord = DbShop & {
  images: ShopImage[];
  courses: ShopCourse[];
  reviews: DbReview[];
};

export const shopInclude = {
  images: true,
  courses: true,
  reviews: true,
} satisfies Prisma.ShopInclude;

const shopDetailSelect = {
  id: true,
  name: true,
  slug: true,
  region: true,
  regionLabel: true,
  subRegion: true,
  subRegionLabel: true,
  theme: true,
  themeLabel: true,
  isPremium: true,
  premiumOrder: true,
  thumbnailUrl: true,
  bannerUrl: true,
  tagline: true,
  description: true,
  address: true,
  phone: true,
  hours: true,
  rating: true,
  reviewCount: true,
  tags: true,
  isVisible: true,
  ownerId: true,
  createdAt: true,
  updatedAt: true,
  images: {
    orderBy: { sortOrder: 'asc' },
    select: {
      imageUrl: true,
      sortOrder: true,
    },
  },
  courses: {
    orderBy: { sortOrder: 'asc' },
    select: {
      name: true,
      durationMinutes: true,
      price: true,
      description: true,
      sortOrder: true,
    },
  },
} satisfies Prisma.ShopSelect;

const shopListSelect = {
  id: true,
  name: true,
  slug: true,
  region: true,
  regionLabel: true,
  subRegion: true,
  subRegionLabel: true,
  theme: true,
  themeLabel: true,
  isPremium: true,
  premiumOrder: true,
  thumbnailUrl: true,
  bannerUrl: true,
  tagline: true,
  rating: true,
  reviewCount: true,
  tags: true,
  createdAt: true,
  updatedAt: true,
  images: {
    orderBy: { sortOrder: 'asc' },
    take: 1,
    select: {
      imageUrl: true,
    },
  },
  courses: {
    orderBy: { sortOrder: 'asc' },
    take: 1,
    select: {
      price: true,
    },
  },
} satisfies Prisma.ShopSelect;

export type ShopDetailRecord = Prisma.ShopGetPayload<{
  select: typeof shopDetailSelect;
}>;

export type ShopListRecord = Prisma.ShopGetPayload<{
  select: typeof shopListSelect;
}>;

const PUBLIC_DIRECTORY_SHOPS_CACHE_TAG = 'public-directory-shops';
const PUBLIC_SHOP_DETAIL_CACHE_TAG = 'public-shop-detail';

function normalizePublicShopSlugCacheKey(slug: string) {
  return slug.trim();
}

const MAX_SEARCH_QUERY_LENGTH = 100;

// 검색어는 인덱스를 못 타는 ILIKE와 캐시 키에 그대로 쓰이므로, 대용량 문자열로 인한
// 쿼리 비용/캐시 증폭을 막기 위해 길이를 제한한다.
function capSearchQuery(query?: string) {
  const trimmed = query?.trim() ?? '';
  return trimmed.length > MAX_SEARCH_QUERY_LENGTH ? trimmed.slice(0, MAX_SEARCH_QUERY_LENGTH) : trimmed;
}

function normalizeDirectoryShopListFilters(filters: DirectoryShopFilters) {
  return {
    region: filters.region ?? '',
    subRegion: filters.subRegion ?? '',
    theme: filters.theme ?? '',
    query: capSearchQuery(filters.query),
    sort: filters.sort ?? '',
    regularOffset: Math.max(0, filters.regularOffset ?? 0),
    regularLimit: filters.regularLimit && filters.regularLimit > 0 ? filters.regularLimit : null,
    includePremium: filters.includePremium !== false,
  };
}

function normalizeDirectoryShopListCacheKey(filters: DirectoryShopFilters) {
  return JSON.stringify(normalizeDirectoryShopListFilters(filters));
}

function normalizeTopShopListFilters(filters: ShopFilters, limit: number) {
  return {
    region: filters.region ?? '',
    subRegion: filters.subRegion ?? '',
    theme: filters.theme ?? '',
    query: capSearchQuery(filters.query),
    limit: Math.max(1, limit),
  };
}

function getRegionVariants(region?: string) {
  if (!region || region === 'all') {
    return [];
  }

  const mappedRegion = REGION_MAP[region] ?? region;
  return mappedRegion === region ? [region] : [region, mappedRegion];
}


function normalizeTopShopListCacheKey(filters: ShopFilters, limit: number) {
  return JSON.stringify(normalizeTopShopListFilters(filters, limit));
}

function isMissingNextCacheContextError(error: unknown) {
  return (
    error instanceof Error &&
    (error.message.includes('static generation store missing') || error.message.includes('incrementalCache missing'))
  );
}

export function invalidatePublicShopListCache() {
  try {
    revalidateTag(PUBLIC_DIRECTORY_SHOPS_CACHE_TAG, 'max');
  } catch (error) {
    if (!isMissingNextCacheContextError(error)) {
      throw error;
    }
  }
}

export function invalidatePublicShopDetailCache() {
  try {
    revalidateTag(PUBLIC_SHOP_DETAIL_CACHE_TAG, 'max');
  } catch (error) {
    if (!isMissingNextCacheContextError(error)) {
      throw error;
    }
  }
}

export function invalidatePublicShopCaches() {
  invalidatePublicShopListCache();
  invalidatePublicShopDetailCache();
  try {
    revalidateTag('admin-dashboard', 'max');
    revalidateTag('managed-shops', 'max');
    revalidateTag('managed-qna', 'max');
  } catch (error) {
    if (!isMissingNextCacheContextError(error)) {
      throw error;
    }
  }
}

export function mapShop(record: ShopRecord): Shop {
  return {
    id: record.id,
    name: record.name,
    slug: record.slug,
    region: record.region,
    regionLabel: record.regionLabel,
    subRegion: record.subRegion ?? undefined,
    subRegionLabel: record.subRegionLabel ?? undefined,
    theme: record.theme,
    themeLabel: record.themeLabel,
    isPremium: record.isPremium,
    premiumOrder: record.premiumOrder ?? undefined,
    thumbnailUrl: record.thumbnailUrl ?? record.images[0]?.imageUrl ?? '',
    bannerUrl: record.bannerUrl ?? record.images[0]?.imageUrl ?? '',
    images: [...record.images]
      .sort((left, right) => left.sortOrder - right.sortOrder)
      .map((image) => image.imageUrl),
    tagline: record.tagline,
    description: record.description,
    address: record.address,
    phone: record.phone,
    hours: record.hours,
    rating: record.rating,
    reviewCount: record.reviewCount,
    courses: [...record.courses]
      .sort((left, right) => left.sortOrder - right.sortOrder)
      .map((course) => ({
        name: course.name,
        duration: `${course.durationMinutes}분`,
        price: `${course.price}`,
        description: course.description ?? undefined,
      })),
    tags: record.tags,
    isVisible: record.isVisible,
    ownerId: record.ownerId ?? undefined,
    createdAt: record.createdAt.toISOString(),
    updatedAt: record.updatedAt.toISOString(),
  };
}

function mapShopDetail(record: ShopDetailRecord): Shop {
  const firstGalleryImage = record.images[0]?.imageUrl ?? '';
  const thumbnailUrl = record.thumbnailUrl
    ? buildShopThumbnailProxyUrl(record.slug, record.updatedAt, 'card')
    : firstGalleryImage
      ? buildShopGalleryImageProxyUrl(record.slug, record.updatedAt, 0, 'card')
      : '';
  const bannerUrl = record.bannerUrl
    ? buildShopBannerProxyUrl(record.slug, record.updatedAt, 'hero')
    : firstGalleryImage
      ? buildShopGalleryImageProxyUrl(record.slug, record.updatedAt, 0, 'hero')
      : '';
  const detailImageUrl = record.thumbnailUrl
    ? buildShopThumbnailProxyUrl(record.slug, record.updatedAt, 'hero')
    : firstGalleryImage
      ? buildShopGalleryImageProxyUrl(record.slug, record.updatedAt, 0, 'hero')
      : bannerUrl;

  return {
    id: record.id,
    name: record.name,
    slug: record.slug,
    region: record.region,
    regionLabel: record.regionLabel,
    subRegion: record.subRegion ?? undefined,
    subRegionLabel: record.subRegionLabel ?? undefined,
    theme: record.theme,
    themeLabel: record.themeLabel,
    isPremium: record.isPremium,
    premiumOrder: record.premiumOrder ?? undefined,
    thumbnailUrl,
    bannerUrl,
    detailImageUrl,
    images: record.images.map((_image, index) => buildShopGalleryImageProxyUrl(record.slug, record.updatedAt, index, 'gallery')),
    tagline: record.tagline,
    description: record.description,
    address: record.address,
    phone: record.phone,
    hours: record.hours,
    rating: record.rating,
    reviewCount: record.reviewCount,
    courses: record.courses.map((course) => ({
      name: course.name,
      duration: `${course.durationMinutes}분`,
      price: `${course.price}`,
      description: course.description ?? undefined,
    })),
    tags: record.tags,
    isVisible: record.isVisible,
    ownerId: record.ownerId ?? undefined,
    createdAt: record.createdAt.toISOString(),
    updatedAt: record.updatedAt.toISOString(),
  };
}

function mapShopList(record: ShopListRecord): ShopListItem {
  const firstGalleryImage = record.images[0]?.imageUrl ?? '';
  const thumbnailUrl = record.thumbnailUrl
    ? buildShopThumbnailProxyUrl(record.slug, record.updatedAt, 'card')
    : firstGalleryImage
      ? buildShopGalleryImageProxyUrl(record.slug, record.updatedAt, 0, 'card')
      : '';
  const bannerUrl = record.bannerUrl
    ? buildShopBannerProxyUrl(record.slug, record.updatedAt, 'hero')
    : firstGalleryImage
      ? buildShopGalleryImageProxyUrl(record.slug, record.updatedAt, 0, 'hero')
      : '';
  const detailImageUrl = record.thumbnailUrl
    ? buildShopThumbnailProxyUrl(record.slug, record.updatedAt, 'hero')
    : firstGalleryImage
      ? buildShopGalleryImageProxyUrl(record.slug, record.updatedAt, 0, 'hero')
      : bannerUrl;

  return {
    id: record.id,
    name: record.name,
    slug: record.slug,
    region: record.region,
    regionLabel: record.regionLabel,
    subRegion: record.subRegion ?? undefined,
    subRegionLabel: record.subRegionLabel ?? undefined,
    theme: record.theme,
    themeLabel: record.themeLabel,
    isPremium: record.isPremium,
    premiumOrder: record.premiumOrder ?? undefined,
    thumbnailUrl,
    bannerUrl,
    detailImageUrl,
    tagline: record.tagline,
    rating: record.rating,
    reviewCount: record.reviewCount,
    courses: record.courses.map((course) => ({
      name: '',
      duration: '',
      price: `${course.price}`,
    })),
    tags: record.tags,
    createdAt: record.createdAt.toISOString(),
  };
}

async function getShopThumbnailSource(slug: string) {
  const shop = await withDatabaseRetry(() =>
    prisma.shop.findUnique({
      where: { slug },
      select: {
        thumbnailUrl: true,
        isVisible: true,
      },
    }),
  );

  if (!shop?.isVisible || !shop.thumbnailUrl) {
    return null;
  }

  return { thumbnailUrl: shop.thumbnailUrl };
}

const getCachedShopThumbnailSource = unstable_cache(
  getShopThumbnailSource,
  [PUBLIC_SHOP_DETAIL_CACHE_TAG, 'thumbnail-source'],
  { revalidate: 120, tags: [PUBLIC_SHOP_DETAIL_CACHE_TAG] },
);

export async function getShopThumbnailBySlug(slug: string) {
  return getCachedShopThumbnailSource(slug);
}

async function getShopBannerSource(slug: string) {
  const shop = await withDatabaseRetry(() =>
    prisma.shop.findUnique({
      where: { slug },
      select: {
        bannerUrl: true,
        isVisible: true,
      },
    }),
  );

  if (!shop?.isVisible || !shop.bannerUrl) {
    return null;
  }

  return { bannerUrl: shop.bannerUrl };
}

const getCachedShopBannerSource = unstable_cache(
  getShopBannerSource,
  [PUBLIC_SHOP_DETAIL_CACHE_TAG, 'banner-source'],
  { revalidate: 120, tags: [PUBLIC_SHOP_DETAIL_CACHE_TAG] },
);

export async function getShopBannerBySlug(slug: string) {
  return getCachedShopBannerSource(slug);
}

async function getShopGalleryImageSource(slug: string, index: number) {
  const shop = await withDatabaseRetry(() =>
    prisma.shop.findUnique({
      where: { slug },
      select: {
        isVisible: true,
        images: {
          orderBy: { sortOrder: 'asc' },
          select: {
            imageUrl: true,
          },
        },
      },
    }),
  );

  if (!shop?.isVisible) {
    return null;
  }

  const source = shop.images[index]?.imageUrl;
  if (!source) {
    return null;
  }

  return { imageUrl: source };
}

const getCachedShopGalleryImageSource = unstable_cache(
  getShopGalleryImageSource,
  [PUBLIC_SHOP_DETAIL_CACHE_TAG, 'gallery-source'],
  { revalidate: 120, tags: [PUBLIC_SHOP_DETAIL_CACHE_TAG] },
);

export async function getShopGalleryImageBySlug(slug: string, index: number) {
  return getCachedShopGalleryImageSource(slug, index);
}

function buildShopWhere(filters: ShopFilters): Prisma.ShopWhereInput {
  const regionVariants = getRegionVariants(filters.region);

  return {
    isVisible: true,
    ...(regionVariants.length === 1
      ? { region: regionVariants[0] }
      : regionVariants.length > 1
        ? { region: { in: regionVariants } }
        : {}),
    ...(filters.subRegion && filters.subRegion !== 'all' ? { subRegion: filters.subRegion } : {}),
    ...(filters.theme && filters.theme !== 'all' ? { theme: filters.theme } : {}),
    ...(filters.query
      ? {
          OR: [{ searchText: { contains: capSearchQuery(filters.query), mode: 'insensitive' } }],
        }
      : {}),
  };
}


function balancePremiumShops(shops: ShopListItem[], region?: string) {
  if (region && region !== 'all') {
    return [...shops].sort((left, right) => (left.premiumOrder ?? 999) - (right.premiumOrder ?? 999));
  }

  const regionGroups = new Map<string, ShopListItem[]>();
  shops.forEach((shop) => {
    const list = regionGroups.get(shop.region) || [];
    list.push(shop);
    regionGroups.set(
      shop.region,
      list.sort((a, b) => (a.premiumOrder ?? 999) - (b.premiumOrder ?? 999)),
    );
  });

  const balanced: ShopListItem[] = [];
  const regions = Array.from(regionGroups.keys());
  const maxLen = Math.max(...Array.from(regionGroups.values()).map((list) => list.length), 0);

  for (let index = 0; index < maxLen; index += 1) {
    for (const currentRegion of regions) {
      const list = regionGroups.get(currentRegion);
      if (list?.[index]) {
        balanced.push(list[index]);
      }
    }
  }

  return balanced;
}

function getRegularOrderBy(sort?: string): Prisma.ShopOrderByWithRelationInput[] {
  if (sort === 'popular') {
    return [{ reviewCount: 'desc' }, { rating: 'desc' }, { createdAt: 'desc' }];
  }

  if (sort === 'new') {
    return [{ createdAt: 'desc' }];
  }

  return [{ createdAt: 'desc' }];
}

function buildTopShopWhereSql(filters: ShopFilters) {
  const regionVariants = getRegionVariants(filters.region);
  const trimmedQuery = filters.query?.trim();
  const conditions: Prisma.Sql[] = [Prisma.sql`s."is_visible" = true`];

  if (regionVariants.length === 1) {
    conditions.push(Prisma.sql`s."region" = ${regionVariants[0]}`);
  } else if (regionVariants.length > 1) {
    conditions.push(Prisma.sql`s."region" IN (${Prisma.join(regionVariants)})`);
  }

  if (filters.subRegion && filters.subRegion !== 'all') {
    conditions.push(Prisma.sql`s."sub_region" = ${filters.subRegion}`);
  }

  if (filters.theme && filters.theme !== 'all') {
    conditions.push(Prisma.sql`s."theme" = ${filters.theme}`);
  }

  if (trimmedQuery) {
    const searchTerm = `%${trimmedQuery}%`;
    conditions.push(Prisma.sql`(
      s."search_text" ILIKE ${searchTerm}
    )`);
  }

  return Prisma.join(conditions, ' AND ');
}

async function listTopShopsUncached(filters: ShopFilters = {}, limit = 100): Promise<ShopListItem[]> {
  const normalizedLimit = Math.max(1, limit);
  const topShopIds = await withDatabaseRetry(() =>
    prisma.$queryRaw<Array<{ id: string }>>(Prisma.sql`
      SELECT s."id"
      FROM "shops" AS s
      WHERE ${buildTopShopWhereSql(filters)}
      ORDER BY s."review_count" DESC, s."rating" DESC, s."created_at" DESC
      LIMIT ${normalizedLimit}
    `),
  );

  if (topShopIds.length === 0) {
    return [];
  }

  const records = await withDatabaseRetry(() =>
    prisma.shop.findMany({
      where: { id: { in: topShopIds.map(({ id }) => id) } },
      select: shopListSelect,
    }),
  );
  const recordMap = new Map(records.map((record) => [record.id, mapShopList(record)]));

  return topShopIds.map(({ id }) => recordMap.get(id)).filter((shop): shop is ShopListItem => Boolean(shop));
}

const getPersistentTopShopList = unstable_cache(
  async (serializedFilters: string) => {
    const normalized = JSON.parse(serializedFilters) as ReturnType<typeof normalizeTopShopListFilters>;
    return listTopShopsUncached(
      {
        region: normalized.region || undefined,
        subRegion: normalized.subRegion || undefined,
        theme: normalized.theme || undefined,
        query: normalized.query || undefined,
      },
      normalized.limit,
    );
  },
  [PUBLIC_DIRECTORY_SHOPS_CACHE_TAG, 'top-shops'],
  { revalidate: 120, tags: [PUBLIC_DIRECTORY_SHOPS_CACHE_TAG] },
);

export async function listTopShops(filters: ShopFilters = {}, limit = 100) {
  const cacheKey = normalizeTopShopListCacheKey(filters, limit);
  try {
    return await getPersistentTopShopList(cacheKey);
  } catch (error) {
    if (isMissingNextCacheContextError(error)) {
      return listTopShopsUncached(filters, limit);
    }

    throw error;
  }
}

async function listDirectoryShopsUncached(filters: DirectoryShopFilters = {}): Promise<ShopListResponse> {
  const regularOffset = Math.max(0, filters.regularOffset ?? 0);
  const regularLimit = filters.regularLimit && filters.regularLimit > 0 ? filters.regularLimit : undefined;
  const includePremium = filters.includePremium !== false;
  const baseWhere = buildShopWhere(filters);
  const premiumWhere: Prisma.ShopWhereInput = {
    ...baseWhere,
    isPremium: true,
  };
  const regularWhere: Prisma.ShopWhereInput = {
    ...baseWhere,
    isPremium: false,
  };

  const [premiumRecords, regularRecords, regularTotal] = await Promise.all([
    includePremium
      ? withDatabaseRetry(() =>
          prisma.shop.findMany({
            where: premiumWhere,
            select: shopListSelect,
            orderBy: [{ premiumOrder: 'asc' }, { createdAt: 'desc' }],
          }),
        )
      : Promise.resolve([] as ShopListRecord[]),
    withDatabaseRetry(() =>
      prisma.shop.findMany({
        where: regularWhere,
        select: shopListSelect,
        orderBy: getRegularOrderBy(filters.sort),
        skip: regularOffset,
        ...(regularLimit ? { take: regularLimit } : {}),
      }),
    ),
    withDatabaseRetry(() => prisma.shop.count({ where: regularWhere })),
  ]);

  const premiumShops = balancePremiumShops(
    premiumRecords.map((shop) => mapShopList(shop)),
    filters.region,
  );
  const regularShops = regularRecords.map((shop) => mapShopList(shop));

  return {
    allShops: [...premiumShops, ...regularShops],
    premiumShops,
    regularShops,
    regularTotal,
    total: premiumShops.length + regularTotal,
  };
}

const getPersistentDirectoryShopList = unstable_cache(
  async (serializedFilters: string) => {
    const normalized = JSON.parse(serializedFilters) as ReturnType<typeof normalizeDirectoryShopListFilters>;
    return listDirectoryShopsUncached({
      region: normalized.region || undefined,
      subRegion: normalized.subRegion || undefined,
      theme: normalized.theme || undefined,
      query: normalized.query || undefined,
      sort: normalized.sort || undefined,
      regularOffset: normalized.regularOffset,
      regularLimit: normalized.regularLimit ?? undefined,
      includePremium: normalized.includePremium,
    });
  },
  [PUBLIC_DIRECTORY_SHOPS_CACHE_TAG],
  { revalidate: 120, tags: [PUBLIC_DIRECTORY_SHOPS_CACHE_TAG] },
);

const directoryShopListInFlight = new Map<string, Promise<ShopListResponse>>();

export async function listDirectoryShops(filters: DirectoryShopFilters = {}) {
  const normalizedQuery = filters.query?.trim();
  const normalizedFilters = normalizedQuery ? { ...filters, query: normalizedQuery } : filters;
  const cacheKey = normalizeDirectoryShopListCacheKey(normalizedFilters);

  return getSharedInFlight(directoryShopListInFlight, cacheKey, async () => {
    if (normalizedQuery) {
      return listDirectoryShopsUncached(normalizedFilters);
    }

    try {
      return await getPersistentDirectoryShopList(cacheKey);
    } catch (error) {
      if (isMissingNextCacheContextError(error)) {
        return listDirectoryShopsUncached(normalizedFilters);
      }

      throw error;
    }
  });
}

const getVisibleShopDetailRecord = cache(async (slug: string) => {
  return withDatabaseRetry(() =>
    prisma.shop.findFirst({
      where: { slug, isVisible: true },
      select: shopDetailSelect,
    }),
  );
});

const getShopBySlugUncached = async (slug: string) => {
  const shop = await getVisibleShopDetailRecord(slug);

  if (!shop) {
    return null;
  }

  return {
    shop: mapShopDetail(shop),
  };
};

const getPersistentShopDetail = unstable_cache(
  async (normalizedSlug: string) => getShopBySlugUncached(normalizedSlug),
  [PUBLIC_SHOP_DETAIL_CACHE_TAG, 'detail'],
  { revalidate: 120, tags: [PUBLIC_SHOP_DETAIL_CACHE_TAG] },
);

export async function getShopBySlug(slug: string) {
  const normalizedSlug = normalizePublicShopSlugCacheKey(slug);

  try {
    return await getPersistentShopDetail(normalizedSlug);
  } catch (error) {
    if (isMissingNextCacheContextError(error)) {
      return getShopBySlugUncached(normalizedSlug);
    }

    throw error;
  }
}

const getShopMetadataBySlugUncached = async (slug: string) => {
  const shop = await getVisibleShopDetailRecord(slug);

  if (!shop) {
    return null;
  }

  return {
    name: shop.name,
    regionLabel: shop.regionLabel,
    themeLabel: shop.themeLabel,
    description: shop.description,
  };
};

const getPersistentShopMetadata = unstable_cache(
  async (normalizedSlug: string) => getShopMetadataBySlugUncached(normalizedSlug),
  [PUBLIC_SHOP_DETAIL_CACHE_TAG, 'metadata'],
  { revalidate: 120, tags: [PUBLIC_SHOP_DETAIL_CACHE_TAG] },
);

export async function getShopMetadataBySlug(slug: string) {
  const normalizedSlug = normalizePublicShopSlugCacheKey(slug);

  try {
    return await getPersistentShopMetadata(normalizedSlug);
  } catch (error) {
    if (isMissingNextCacheContextError(error)) {
      return getShopMetadataBySlugUncached(normalizedSlug);
    }

    throw error;
  }
}

const getVisibleShopSlugsUncached = async () => {
  const shops = await withDatabaseRetry(() =>
    prisma.shop.findMany({
      where: { isVisible: true },
      select: { slug: true },
      orderBy: [{ isPremium: 'desc' }, { premiumOrder: 'asc' }, { createdAt: 'desc' }],
    }),
  );

  return shops.map((shop) => shop.slug);
};

const getPersistentVisibleShopSlugs = unstable_cache(
  async () => getVisibleShopSlugsUncached(),
  [PUBLIC_SHOP_DETAIL_CACHE_TAG, 'slugs'],
  { revalidate: 120, tags: [PUBLIC_SHOP_DETAIL_CACHE_TAG] },
);

export async function listVisibleShopSlugs() {
  try {
    return await getPersistentVisibleShopSlugs();
  } catch (error) {
    if (isMissingNextCacheContextError(error)) {
      return getVisibleShopSlugsUncached();
    }

    throw error;
  }
}
const SHOP_DETAIL_WARM_CONCURRENCY = 2;
const pendingShopDetailWarmSlugs = new Set<string>();
let activeShopDetailWarmPromise: Promise<void> | null = null;

function startShopDetailWarmQueue() {
  if (activeShopDetailWarmPromise) {
    return activeShopDetailWarmPromise;
  }

  activeShopDetailWarmPromise = (async () => {
    while (pendingShopDetailWarmSlugs.size > 0) {
      const slugs = Array.from(pendingShopDetailWarmSlugs);
      pendingShopDetailWarmSlugs.clear();

      await runWithConcurrencyLimit(
        slugs.flatMap((slug) => [
          async () => {
            await getShopBySlug(slug);
          },
          async () => {
            await getShopMetadataBySlug(slug);
          },
        ]),
        SHOP_DETAIL_WARM_CONCURRENCY,
      );
    }
  })().finally(() => {
    activeShopDetailWarmPromise = null;
  });

  return activeShopDetailWarmPromise;
}


export async function warmPublicShopDetailCaches(slugs: string[]) {
  const normalizedSlugs = Array.from(
    new Set(slugs.map((slug) => normalizePublicShopSlugCacheKey(slug)).filter(Boolean)),
  );

  for (const slug of normalizedSlugs) {
    pendingShopDetailWarmSlugs.add(slug);
  }

  await startShopDetailWarmQueue();
}
const SHOP_DETAIL_REVIEWS_LIMIT = 100;

export async function getShopReviewsBySlug(slug: string): Promise<Review[]> {
  const reviews = await withDatabaseRetry(() =>
    prisma.review.findMany({
      where: {
        isHidden: false,
        shop: {
          slug,
          isVisible: true,
        },
      },
      orderBy: { createdAt: 'desc' },
      take: SHOP_DETAIL_REVIEWS_LIMIT,
      select: {
        id: true,
        shopId: true,
        authorName: true,
        rating: true,
        content: true,
        createdAt: true,
        shop: {
          select: {
            name: true,
          },
        },
      },
    }),
  );

  return reviews.map((review) => ({
    id: review.id,
    shopId: review.shopId,
    shopName: review.shop.name,
    authorName: review.authorName,
    rating: review.rating,
    content: review.content,
    createdAt: review.createdAt.toISOString(),
  }));
}

export async function updateShopVisibility(shopId: string, isVisible: boolean) {
  try {
    const shop = await withDatabaseRetry(() =>
      prisma.shop.update({
        where: { id: shopId },
        data: { isVisible },
        include: shopInclude,
      }),
    );
    invalidatePublicShopCaches();
    return mapShop(shop);
  } catch (error) {
    if (isRecordNotFoundError(error)) {
      return null;
    }
    console.error('Failed to update shop visibility:', error);
    throw new Error('DATABASE_ERROR');
  }
}

export async function updateShopPremium(shopId: string, isPremium: boolean, premiumOrder?: number) {
  try {
    const shop = await withDatabaseRetry(() =>
      prisma.shop.update({
        where: { id: shopId },
        data: {
          isPremium,
          premiumOrder: isPremium ? premiumOrder ?? 1 : null,
        },
        include: shopInclude,
      }),
    );
    invalidatePublicShopCaches();
    return mapShop(shop);
  } catch (error) {
    if (isRecordNotFoundError(error)) {
      return null;
    }
    console.error('Failed to update shop premium:', error);
    throw new Error('DATABASE_ERROR');
  }
}
