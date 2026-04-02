import { useState, useCallback, useRef, useEffect } from 'react';

/**
 * JustifiedGrid — a "Google Photos"-style horizontal masonry layout.
 *
 * Images are arranged into rows, each filled to the container width.
 * Each row's images are scaled to the same height so they tile perfectly.
 *
 * For images without known dimensions, a default aspect ratio (4:3) is used
 * until the image loads and reports its natural size.
 */

interface JustifiedItem {
  id: string | number;
  src: string;
  thumbSrc?: string;
  width?: number;
  height?: number;
  /** Overlay rendered on top of the image (visible on hover). */
  overlay?: React.ReactNode;
  /** Overlay always visible regardless of hover state. */
  persistentOverlay?: React.ReactNode;
}

interface JustifiedGridProps {
  items: JustifiedItem[];
  /** Target row height in pixels. Default: 220 */
  rowHeight?: number;
  /** Gap between items in pixels. Default: 4 */
  gap?: number;
  /** Called when an item is clicked. */
  onItemClick?: (index: number) => void;
}

const DEFAULT_ASPECT = 4 / 3;

export function JustifiedGrid({
  items,
  rowHeight = 220,
  gap = 4,
  onItemClick,
}: JustifiedGridProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(0);

  // Track actual aspect ratios as images load (for items without dimensions).
  const [aspects, setAspects] = useState<Map<string | number, number>>(new Map());

  useEffect(() => {
    if (!containerRef.current) return;

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
      }
    });
    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, []);

  const handleImageLoad = useCallback(
    (id: string | number, e: React.SyntheticEvent<HTMLImageElement>) => {
      const img = e.currentTarget;
      if (img.naturalWidth && img.naturalHeight) {
        setAspects((prev) => {
          const next = new Map(prev);
          next.set(id, img.naturalWidth / img.naturalHeight);
          return next;
        });
      }
    },
    [],
  );

  // Compute aspect ratio for each item.
  const getAspect = useCallback(
    (item: JustifiedItem): number => {
      if (item.width && item.height) return item.width / item.height;
      return aspects.get(item.id) ?? DEFAULT_ASPECT;
    },
    [aspects],
  );

  // Build rows.
  const rows = layoutRows(items, containerWidth, rowHeight, gap, getAspect);

  return (
    <div ref={containerRef} className="w-full">
      {containerWidth > 0 &&
        rows.map((row, rowIdx) => (
          <div
            key={rowIdx}
            className="flex"
            style={{ gap, marginBottom: gap }}
          >
            {row.cells.map((cell) => (
              <div
                key={cell.item.id}
                className="relative overflow-hidden rounded-sm bg-zinc-800 group cursor-pointer"
                style={{
                  width: cell.width,
                  height: row.height,
                  flexShrink: 0,
                }}
                onClick={() => onItemClick?.(cell.globalIndex)}
              >
                <img
                  src={cell.item.thumbSrc ?? cell.item.src}
                  alt=""
                  className="w-full h-full object-cover"
                  loading="lazy"
                  onLoad={(e) => handleImageLoad(cell.item.id, e)}
                  onError={(e) => {
                    // Fall back to full-size src if thumbnail fails.
                    const target = e.currentTarget;
                    if (
                      cell.item.thumbSrc &&
                      target.src !== window.location.origin + cell.item.src
                    ) {
                      target.src = cell.item.src;
                    }
                  }}
                />
                {cell.item.persistentOverlay && (
                  <div className="absolute inset-0 pointer-events-none">
                    {cell.item.persistentOverlay}
                  </div>
                )}
                {cell.item.overlay && (
                  <div className="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity">
                    {cell.item.overlay}
                  </div>
                )}
              </div>
            ))}
          </div>
        ))}
    </div>
  );
}

// ----- Layout engine -----

interface LayoutCell {
  item: JustifiedItem;
  width: number;
  globalIndex: number;
}

interface LayoutRow {
  cells: LayoutCell[];
  height: number;
}

function layoutRows(
  items: JustifiedItem[],
  containerWidth: number,
  targetHeight: number,
  gap: number,
  getAspect: (item: JustifiedItem) => number,
): LayoutRow[] {
  if (containerWidth <= 0 || items.length === 0) return [];

  const rows: LayoutRow[] = [];
  let currentRow: { item: JustifiedItem; aspect: number; globalIndex: number }[] = [];
  let currentAspectSum = 0;

  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    const aspect = getAspect(item);
    currentRow.push({ item, aspect, globalIndex: i });
    currentAspectSum += aspect;

    // Width that this row would take at the target height.
    const gapSpace = (currentRow.length - 1) * gap;
    const naturalWidth = currentAspectSum * targetHeight + gapSpace;

    if (naturalWidth >= containerWidth) {
      // This row is full. Compute actual height so items exactly fill the width.
      const actualHeight = (containerWidth - gapSpace) / currentAspectSum;
      rows.push({
        height: actualHeight,
        cells: currentRow.map((c) => ({
          item: c.item,
          width: c.aspect * actualHeight,
          globalIndex: c.globalIndex,
        })),
      });
      currentRow = [];
      currentAspectSum = 0;
    }
  }

  // Last partial row — show at target height (don't stretch).
  if (currentRow.length > 0) {
    const gapSpace = (currentRow.length - 1) * gap;
    const naturalWidth = currentAspectSum * targetHeight + gapSpace;
    // If the partial row is reasonably full (>60%), justify it. Otherwise, keep target height.
    const actualHeight =
      naturalWidth >= containerWidth * 0.6
        ? (containerWidth - gapSpace) / currentAspectSum
        : targetHeight;
    rows.push({
      height: actualHeight,
      cells: currentRow.map((c) => ({
        item: c.item,
        width: c.aspect * actualHeight,
        globalIndex: c.globalIndex,
      })),
    });
  }

  return rows;
}

export type { JustifiedItem };
