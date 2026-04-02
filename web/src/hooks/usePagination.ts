import { useCallback, useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';

interface UsePaginationOptions {
  /** Items per page. */
  limit?: number;
  /** URL search param name for the page number. Defaults to "page". */
  paramName?: string;
}

interface UsePaginationResult {
  /** Current page number (1-indexed). */
  page: number;
  /** Items per page. */
  limit: number;
  /** Offset to pass to the API. */
  offset: number;
  /** Navigate to a specific page (1-indexed). */
  setPage: (page: number) => void;
  /** Go to the previous page. No-op on page 1. */
  prevPage: () => void;
  /** Go to the next page. */
  nextPage: () => void;
  /** Reset to page 1. Useful when filters change. */
  resetPage: () => void;
}

/**
 * Pagination hook backed by URL search params.
 *
 * Page numbers in the URL are 1-indexed (`?page=2`).
 * Page 1 omits the param for cleaner URLs.
 */
export function usePagination(options: UsePaginationOptions = {}): UsePaginationResult {
  const { limit = 50, paramName = 'page' } = options;
  const [searchParams, setSearchParams] = useSearchParams();

  const page = useMemo(() => {
    const raw = searchParams.get(paramName);
    const parsed = raw ? parseInt(raw, 10) : 1;
    return parsed >= 1 ? parsed : 1;
  }, [searchParams, paramName]);

  const offset = (page - 1) * limit;

  const setPage = useCallback(
    (p: number) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (p <= 1) {
            next.delete(paramName);
          } else {
            next.set(paramName, String(p));
          }
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams, paramName],
  );

  const prevPage = useCallback(() => {
    if (page > 1) setPage(page - 1);
  }, [page, setPage]);

  const nextPage = useCallback(() => {
    setPage(page + 1);
  }, [page, setPage]);

  const resetPage = useCallback(() => {
    setPage(1);
  }, [setPage]);

  return { page, limit, offset, setPage, prevPage, nextPage, resetPage };
}
