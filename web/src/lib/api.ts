// Typed API client for the GoDownload backend.

import type {
  ActiveDownload,
  AdminStats,
  ColorSearchResult,
  DownloadQueue,
  EnrichResult,
  Gallery,
  GallerySearchResult,
  IdentifyRequest,
  Image,
  PaginationParams,
  Person,
  PersonIdentifier,
  ProviderSearchResponse,
  QueueStatus,
  Source,
} from '@/types';

const BASE = '/api/v1';

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    ...init,
  });
  if (res.status === 204) return undefined as T;
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new ApiError(res.status, body.error ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

function qs(params: Record<string, string | number | boolean | undefined> | object): string {
  const entries = Object.entries(params).filter(([, v]) => v !== undefined);
  if (entries.length === 0) return '';
  return '?' + new URLSearchParams(entries.map(([k, v]) => [k, String(v)])).toString();
}

// ---------- Sources ----------

export const sources = {
  list: () => request<Source[]>('/sources'),
  create: (data: { url: string; name: string; priority?: number }) =>
    request<Source>('/sources', { method: 'POST', body: JSON.stringify(data) }),
  crawl: (id: number) =>
    request<{ message: string }>(`/sources/${id}/crawl`, { method: 'POST' }),
  recrawl: (id: number) =>
    request<{ message: string }>(`/sources/${id}/recrawl`, { method: 'POST' }),
  delete: (id: number) =>
    request<void>(`/sources/${id}`, { method: 'DELETE' }),
};

// ---------- Galleries ----------

export interface GalleryListParams extends PaginationParams {
  source_id?: number;
  search?: string;
}

export const galleries = {
  list: (params: GalleryListParams = {}) =>
    request<PaginatedResult<Gallery>>(`/galleries${qs(params)}`),
  get: (id: number) => request<Gallery>(`/galleries/${id}`),
  people: (id: number) => request<Person[]>(`/galleries/${id}/people`),
  create: (data: Partial<Gallery>) =>
    request<Gallery>('/galleries', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: number, data: Partial<Gallery>) =>
    request<Gallery>(`/galleries/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: number) => request<void>(`/galleries/${id}`, { method: 'DELETE' }),
  searchMetadata: (id: number, query: string, provider?: string) =>
    request<GallerySearchResult[]>(`/galleries/${id}/search-metadata${qs({ query, provider })}`),
  scrapeMetadata: (id: number, data: { provider: string; url: string; source_id?: string }) =>
    request<Gallery>(`/galleries/${id}/scrape-metadata`, { method: 'POST', body: JSON.stringify(data) }),
  metadataProviders: () => request<string[]>('/galleries/metadata-providers'),
};

// ---------- Images ----------

export interface ImageListParams extends PaginationParams {
  gallery_id?: number;
  is_video?: boolean;
  is_favorite?: boolean;
  sort_by?: 'newest' | 'oldest' | 'largest' | 'smallest' | 'random';
  random_seed?: number;
  on_disk?: boolean;
}

export const images = {
  list: (params: ImageListParams = {}) =>
    request<PaginatedResult<Image>>(`/images${qs(params)}`),
  get: (id: number) => request<Image>(`/images/${id}`),
  delete: (id: number) => request<void>(`/images/${id}`, { method: 'DELETE' }),
  toggleFavorite: (id: number) =>
    request<{ id: number; is_favorite: boolean }>(`/images/${id}/favorite`, { method: 'POST' }),
  redownload: (id: number) =>
    request<{ message: string; image_id: number; queue_id: number }>(
      `/images/${id}/redownload`, { method: 'POST' },
    ),
  searchByColor: (color: string, limit?: number, maxDistance?: number) =>
    request<ColorSearchResult[]>(
      `/images/search/color${qs({ color, limit, max_distance: maxDistance })}`,
    ),
};

// ---------- Videos ----------

export const videos = {
  list: (params: ImageListParams = {}) =>
    request<Image[]>(`/videos${qs(params)}`),
  delete: (id: number) => request<void>(`/images/${id}`, { method: 'DELETE' }),
};

// ---------- People ----------

export interface PeopleListParams extends PaginationParams {
  search?: string;
}

export const people = {
  list: (params: PeopleListParams = {}) =>
    request<PaginatedResult<Person>>(`/people${qs(params)}`),
  get: (id: number) => request<Person>(`/people/${id}`),
  create: (data: { name: string; aliases?: string; nationality?: string }) =>
    request<Person>('/people', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: number, data: Partial<Person>) =>
    request<Person>(`/people/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: number) => request<void>(`/people/${id}`, { method: 'DELETE' }),
  galleries: (id: number, params: PaginationParams = {}) =>
    request<Gallery[]>(`/people/${id}/galleries${qs(params)}`),
  linkGallery: (personId: number, galleryId: number) =>
    request<{ message: string }>(`/people/${personId}/link-gallery/${galleryId}`, { method: 'POST' }),
  unlinkGallery: (personId: number, galleryId: number) =>
    request<{ message: string }>(`/people/${personId}/unlink-gallery/${galleryId}`, { method: 'POST' }),
  identifiers: (id: number) =>
    request<PersonIdentifier[]>(`/people/${id}/identifiers`),
  upsertIdentifier: (id: number, data: { provider: string; external_id: string }) =>
    request<PersonIdentifier>(`/people/${id}/identifiers`, { method: 'POST', body: JSON.stringify(data) }),
  enrich: (id: number, provider?: string, apply?: boolean) =>
    request<EnrichResult | Person>(`/people/${id}/enrich${qs({ provider, apply })}`),
  bulkEnrich: (personIds: number[], provider?: string, apply?: boolean) =>
    request<{ enriched: number; failed: number }>(
      '/people/bulk/enrich',
      { method: 'POST', body: JSON.stringify({ person_ids: personIds, provider, apply }) },
    ),
  bulkMerge: (keepId: number, mergeIds: number[]) =>
    request<{ message: string; person: Person; merged: number }>(
      '/people/bulk/merge',
      { method: 'POST', body: JSON.stringify({ keep_id: keepId, merge_ids: mergeIds }) },
    ),
  bulkDelete: (personIds: number[]) =>
    request<{ deleted: number }>(
      '/people/bulk',
      { method: 'DELETE', body: JSON.stringify({ person_ids: personIds }) },
    ),
  providers: () => request<string[]>('/people/providers'),
  search: (id: number, provider: string, query?: string) =>
    request<ProviderSearchResponse>(`/people/${id}/search${qs({ provider, query })}`),
  identify: (id: number, data: IdentifyRequest) =>
    request<{ message: string; person: Person; identifier: PersonIdentifier }>(
      `/people/${id}/identify`,
      { method: 'POST', body: JSON.stringify(data) },
    ),
};

// ---------- Admin ----------

export const admin = {
  stats: () => request<AdminStats>('/admin/stats'),
  queue: {
    list: (params: PaginationParams & { status?: string; type?: string } = {}) =>
      request<DownloadQueue[]>(`/admin/queue${qs(params)}`),
    status: () => request<QueueStatus>('/admin/queue/status'),
    activeDownloads: () => request<ActiveDownload[]>('/admin/queue/active'),
    pause: () => request<{ message: string; paused: boolean }>('/admin/queue/pause', { method: 'POST' }),
    resume: () => request<{ message: string; paused: boolean }>('/admin/queue/resume', { method: 'POST' }),
    clear: (status?: string) =>
      request<{ deleted: number }>(`/admin/queue${qs({ status })}`, { method: 'DELETE' }),
    retry: (id: number) =>
      request<{ message: string }>(`/admin/queue/${id}/retry`, { method: 'POST' }),
    retryFailed: () =>
      request<{ retried: number }>('/admin/queue/retry-failed', { method: 'POST' }),
    delete: (id: number) =>
      request<void>(`/admin/queue/${id}`, { method: 'DELETE' }),
  },
  prioritizeSource: (id: number) =>
    request<{ message: string; bumped_items: number }>(`/admin/sources/${id}/prioritize`, { method: 'POST' }),
  recrawlSource: (id: number) =>
    request<{ message: string }>(`/admin/sources/${id}/recrawl`, { method: 'POST' }),
  bulkRedownload: (imageIds: number[]) =>
    request<{ enqueued: number }>('/admin/images/redownload', {
      method: 'POST',
      body: JSON.stringify({ image_ids: imageIds }),
    }),
  galleryCleanup: (dryRun = true) =>
    request<{ dry_run: boolean; count?: number; deleted?: number }>(
      `/admin/galleries/cleanup?dry_run=${dryRun}`,
      { method: 'POST' },
    ),
  autolinkGalleries: () =>
    request<{ message: string; linked: number }>('/admin/galleries/autolink', { method: 'POST' }),
  stopServer: () =>
    request<{ message: string }>('/admin/server/stop', {
      method: 'POST',
      body: JSON.stringify({ confirm: 'STOP' }),
    }),
};
