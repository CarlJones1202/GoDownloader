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
