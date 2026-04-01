// API response and model types matching the Go backend.

export interface Source {
  id: number;
  url: string;
  name: string;
  enabled: boolean;
  priority: number;
  last_crawled_at?: string;
  created_at: string;
}

export interface Gallery {
  id: number;
  source_id?: number;
  provider?: string;
  provider_gallery_id?: string;
  title?: string;
  url?: string;
  thumbnail_url?: string;
  local_thumbnail_path?: string;
  created_at: string;
}

export interface Image {
  id: number;
  gallery_id?: number;
  filename: string;
  original_url?: string;
  width?: number;
  height?: number;
  duration_seconds?: number;
  file_hash?: string;
  dominant_colors?: string;
  is_video: boolean;
  vr_mode: string;
  is_favorite: boolean;
  created_at: string;
}

export interface Person {
  id: number;
  name: string;
  aliases?: string;
  birth_date?: string;
  nationality?: string;
  created_at: string;
}

export interface PersonIdentifier {
  id: number;
  person_id: number;
  provider: string;
  external_id: string;
  created_at: string;
}

export interface DownloadQueue {
  id: number;
  type: string;
  url: string;
  target_id?: number;
  status: string;
  retry_count: number;
  error_message?: string;
  created_at: string;
}

export interface QueueStats {
  pending: number;
  active: number;
  completed: number;
  failed: number;
  paused: number;
}

export interface DownloadStats {
  completed_today: number;
  completed_week: number;
  failed_today: number;
  failed_week: number;
}

export interface ProviderBreakdown {
  provider: string;
  count: number;
}

export interface AdminStats {
  sources: number;
  galleries: number;
  images: number;
  videos: number;
  people: number;
  favorites: number;
  queue: QueueStats;
  downloads: DownloadStats;
  provider_breakdown: ProviderBreakdown[];
}

export interface QueueStatus {
  paused: boolean;
  stats: QueueStats;
}

export interface ColorSearchResult {
  image: Image;
  distance: number;
}

export interface EnrichResult {
  merged: PersonInfo;
  providers: Record<string, PersonInfo>;
}

export interface PersonInfo {
  name?: string;
  aliases?: string[];
  birth_date?: string;
  nationality?: string;
  external_id?: string;
}

export interface PaginationParams {
  limit?: number;
  offset?: number;
}
