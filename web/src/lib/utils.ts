// Shared UI utility: className merger.
export function cn(...classes: (string | false | null | undefined)[]): string {
  return classes.filter(Boolean).join(' ');
}

// Format a date string for display.
export function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

// Format a date string with time.
export function formatDateTime(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

// Parse a dominant_colors JSON string into an array of hex strings.
export function parseColors(colorsJson: string | undefined): string[] {
  if (!colorsJson) return [];
  try {
    return JSON.parse(colorsJson) as string[];
  } catch {
    return [];
  }
}

// Build the thumbnail URL for an image filename.
// Convention: "photo.jpg" -> "/data/thumbnails/photo_thumb.jpg"
export function thumbnailUrl(filename: string): string {
  const dot = filename.lastIndexOf('.');
  const base = dot >= 0 ? filename.substring(0, dot) : filename;
  return `/data/thumbnails/${base}_thumb.jpg`;
}

// Build the video URL for a video filename.
export function videoUrl(filename: string): string {
  return `/data/videos/${filename}`;
}

// Build the trickplay VTT URL for a video filename.
// Convention: "abc123.mp4" -> "/data/thumbnails/abc123_sprites.vtt"
export function trickplayVttUrl(filename: string): string {
  const dot = filename.lastIndexOf('.');
  const base = dot >= 0 ? filename.substring(0, dot) : filename;
  return `/data/thumbnails/${base}_sprites.vtt`;
}

// Format seconds into "M:SS" or "H:MM:SS" display string.
export function formatDuration(seconds: number | undefined): string {
  if (!seconds || seconds <= 0) return '0:00';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }
  return `${m}:${String(s).padStart(2, '0')}`;
}
