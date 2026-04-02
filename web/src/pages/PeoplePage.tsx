import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { people } from '@/lib/api';
import { formatDate } from '@/lib/utils';
import {
  PageHeader,
  Button,
  Card,
  Badge,
  Spinner,
  EmptyState,
  Input,
  Pagination,
  ConfirmDialog,
} from '@/components/UI';
import { Plus, Search, Trash2, Users, Sparkles, Merge, ChevronRight } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

export function PeoplePage() {
  const queryClient = useQueryClient();
  const { page, offset, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 50 });
  const [search, setSearch] = useState('');
  const [activeSearch, setActiveSearch] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [newPerson, setNewPerson] = useState({ name: '', aliases: '', nationality: '' });
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [bulkAction, setBulkAction] = useState<'enrich' | 'merge' | 'delete' | null>(null);
  const [mergeKeepId, setMergeKeepId] = useState('');
  const [confirmDelete, setConfirmDelete] = useState(false);

  const { data: personList, isLoading } = useQuery({
    queryKey: ['people', { offset, limit, search: activeSearch || undefined }],
    queryFn: () => people.list({ limit, offset, search: activeSearch || undefined }),
  });

  const createMut = useMutation({
    mutationFn: () =>
      people.create({
        name: newPerson.name,
        aliases: newPerson.aliases || undefined,
        nationality: newPerson.nationality || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['people'] });
      setShowCreate(false);
      setNewPerson({ name: '', aliases: '', nationality: '' });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => people.delete(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['people'] }),
  });

  const bulkEnrichMut = useMutation({
    mutationFn: (ids: number[]) => people.bulkEnrich(ids, undefined, true),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['people'] });
      setSelected(new Set());
      setBulkAction(null);
    },
  });

  const bulkMergeMut = useMutation({
    mutationFn: ({ keepId, mergeIds }: { keepId: number; mergeIds: number[] }) =>
      people.bulkMerge(keepId, mergeIds),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['people'] });
      setSelected(new Set());
      setBulkAction(null);
      setMergeKeepId('');
    },
  });

  const bulkDeleteMut = useMutation({
    mutationFn: (ids: number[]) => people.bulkDelete(ids),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['people'] });
      setSelected(new Set());
      setBulkAction(null);
      setConfirmDelete(false);
    },
  });

  const handleSearch = () => {
    setActiveSearch(search);
    resetPage();
  };

  const toggleSelect = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (!personList) return;
    if (selected.size === personList.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(personList.map((p) => p.id)));
    }
  };

  const selectedIds = Array.from(selected);

  return (
    <>
      <PageHeader title="People" description="Manage performers and metadata">
        <Button onClick={() => setShowCreate(!showCreate)}>
          <Plus size={14} /> Add Person
        </Button>
      </PageHeader>

      {/* Search */}
      <div className="flex gap-2 mb-4">
        <div className="flex-1">
          <Input
            placeholder="Search by name or alias..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
          />
        </div>
        <Button size="sm" onClick={handleSearch}>
          <Search size={14} /> Search
        </Button>
        {activeSearch && (
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              setSearch('');
              setActiveSearch('');
              resetPage();
            }}
          >
            Clear
          </Button>
        )}
      </div>

      {/* Create form */}
      {showCreate && (
        <Card className="mb-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <Input
              label="Name"
              placeholder="Person name"
              value={newPerson.name}
              onChange={(e) => setNewPerson({ ...newPerson, name: e.target.value })}
            />
            <Input
              label="Aliases"
              placeholder="Comma-separated aliases"
              value={newPerson.aliases}
              onChange={(e) => setNewPerson({ ...newPerson, aliases: e.target.value })}
            />
            <Input
              label="Nationality"
              placeholder="e.g. American"
              value={newPerson.nationality}
              onChange={(e) => setNewPerson({ ...newPerson, nationality: e.target.value })}
            />
          </div>
          <div className="flex justify-end gap-2 mt-3">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={() => createMut.mutate()}
              disabled={!newPerson.name || createMut.isPending}
            >
              Create
            </Button>
          </div>
        </Card>
      )}

      {/* Bulk actions */}
      {selected.size > 0 && (
        <Card className="mb-4">
          <div className="flex items-center gap-3 flex-wrap">
            <span className="text-sm text-zinc-300">
              {selected.size} selected
            </span>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setBulkAction('enrich')}
              disabled={bulkEnrichMut.isPending}
            >
              <Sparkles size={14} /> Enrich All
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setBulkAction('merge')}
              disabled={selected.size < 2}
            >
              <Merge size={14} /> Merge
            </Button>
            <Button
              variant="danger"
              size="sm"
              onClick={() => setConfirmDelete(true)}
            >
              <Trash2 size={14} /> Delete
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setSelected(new Set())}
            >
              Clear Selection
            </Button>
          </div>

          {/* Merge UI */}
          {bulkAction === 'merge' && (
            <div className="mt-3 flex items-end gap-2">
              <Input
                label="Keep ID (primary)"
                placeholder="Person ID to keep"
                value={mergeKeepId}
                onChange={(e) => setMergeKeepId(e.target.value)}
                className="w-40"
              />
              <Button
                size="sm"
                onClick={() => {
                  const keepId = parseInt(mergeKeepId);
                  if (!keepId) return;
                  const mergeIds = selectedIds.filter((id) => id !== keepId);
                  bulkMergeMut.mutate({ keepId, mergeIds });
                }}
                disabled={!mergeKeepId || bulkMergeMut.isPending}
              >
                Merge into #{mergeKeepId || '?'}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setBulkAction(null)}>
                Cancel
              </Button>
            </div>
          )}

          {/* Enrich confirmation */}
          {bulkAction === 'enrich' && (
            <div className="mt-3 flex items-center gap-2">
              <span className="text-sm text-zinc-400">
                Enrich {selected.size} people from all providers and apply changes?
              </span>
              <Button
                size="sm"
                onClick={() => bulkEnrichMut.mutate(selectedIds)}
                disabled={bulkEnrichMut.isPending}
              >
                {bulkEnrichMut.isPending ? 'Enriching...' : 'Confirm'}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setBulkAction(null)}>
                Cancel
              </Button>
            </div>
          )}
        </Card>
      )}

      {/* People list */}
      {isLoading ? (
        <Spinner />
      ) : !personList || personList.length === 0 ? (
        <EmptyState message="No people found." />
      ) : (
        <>
          {/* Select all toggle */}
          <div className="flex items-center gap-2 mb-2">
            <input
              type="checkbox"
              checked={personList.length > 0 && selected.size === personList.length}
              onChange={toggleSelectAll}
              className="rounded border-zinc-600 bg-zinc-800 text-blue-500"
            />
            <span className="text-xs text-zinc-500">Select all</span>
          </div>

          <div className="space-y-2">
            {personList.map((p) => (
              <Card key={p.id} className="flex items-center gap-3">
                <input
                  type="checkbox"
                  checked={selected.has(p.id)}
                  onChange={() => toggleSelect(p.id)}
                  className="rounded border-zinc-600 bg-zinc-800 text-blue-500 shrink-0"
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <Users size={14} className="text-zinc-500 shrink-0" />
                    <Link
                      to={`/people/${p.id}`}
                      className="text-sm font-medium text-white hover:text-blue-400 truncate"
                    >
                      {p.name}
                    </Link>
                    {p.nationality && <Badge>{p.nationality}</Badge>}
                  </div>
                  {p.aliases && (
                    <p className="text-xs text-zinc-500 truncate mt-0.5">
                      aka: {p.aliases}
                    </p>
                  )}
                  <p className="text-xs text-zinc-600 mt-0.5">
                    Added: {formatDate(p.created_at)}
                  </p>
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  <Button
                    variant="ghost"
                    size="sm"
                    title="Delete"
                    onClick={() => deleteMut.mutate(p.id)}
                  >
                    <Trash2 size={14} />
                  </Button>
                  <Link to={`/people/${p.id}`}>
                    <Button variant="ghost" size="sm">
                      <ChevronRight size={14} />
                    </Button>
                  </Link>
                </div>
              </Card>
            ))}
          </div>

          <Pagination
            page={page}
            hasMore={personList.length === limit}
            onPrev={prevPage}
            onNext={nextPage}
          />
        </>
      )}

      <ConfirmDialog
        open={confirmDelete}
        title="Bulk Delete"
        message={`Are you sure you want to delete ${selected.size} people? This cannot be undone.`}
        onConfirm={() => bulkDeleteMut.mutate(selectedIds)}
        onCancel={() => setConfirmDelete(false)}
      />
    </>
  );
}
