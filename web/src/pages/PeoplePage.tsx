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
import { Plus, Search, Trash2, User, Sparkles, Merge, ChevronRight } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try {
    const parsed = JSON.parse(photos);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function ProfileTile({ name, photo }: { name: string; photo?: string }) {
  return photo ? (
    <img src={photo} alt={name} className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.03]" />
  ) : (
    <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-zinc-800 to-zinc-950">
      <User size={44} className="text-zinc-600" />
    </div>
  );
}

export function PeoplePage() {
  const queryClient = useQueryClient();
  const { page, offset, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 24 });
  const [search, setSearch] = useState('');
  const [activeSearch, setActiveSearch] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [newPerson, setNewPerson] = useState({ name: '', aliases: '', nationality: '' });
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [bulkAction, setBulkAction] = useState<'enrich' | 'merge' | null>(null);
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
      <PageHeader title="People" description="Profiles and performer metadata">
        <Button onClick={() => setShowCreate(!showCreate)}>
          <Plus size={14} /> Add Person
        </Button>
      </PageHeader>

      <div className="mb-6 rounded-[2rem] border border-white/8 bg-white/5 p-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-1 gap-2">
            <Input
              placeholder="Search profiles, aliases, or details..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            />
            <Button size="sm" onClick={handleSearch}><Search size={14} /> Search</Button>
            {activeSearch && <Button variant="secondary" size="sm" onClick={() => { setSearch(''); setActiveSearch(''); resetPage(); }}>Clear</Button>}
          </div>

          {selected.size > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm text-zinc-400">{selected.size} selected</span>
              <Button variant="secondary" size="sm" onClick={() => setBulkAction('enrich')} disabled={bulkEnrichMut.isPending}>
                <Sparkles size={14} /> Enrich
              </Button>
              <Button variant="secondary" size="sm" onClick={() => setBulkAction('merge')} disabled={selected.size < 2}>
                <Merge size={14} /> Merge
              </Button>
              <Button variant="danger" size="sm" onClick={() => setConfirmDelete(true)}>
                <Trash2 size={14} /> Delete
              </Button>
            </div>
          )}
        </div>
      </div>

      {showCreate && (
        <Card className="mb-6 rounded-[1.75rem] border-white/8 bg-white/5">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <Input label="Name" placeholder="Person name" value={newPerson.name} onChange={(e) => setNewPerson({ ...newPerson, name: e.target.value })} />
            <Input label="Aliases" placeholder="Comma-separated aliases" value={newPerson.aliases} onChange={(e) => setNewPerson({ ...newPerson, aliases: e.target.value })} />
            <Input label="Nationality" placeholder="e.g. American" value={newPerson.nationality} onChange={(e) => setNewPerson({ ...newPerson, nationality: e.target.value })} />
          </div>
          <div className="flex justify-end gap-2 mt-3">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)}>Cancel</Button>
            <Button size="sm" onClick={() => createMut.mutate()} disabled={!newPerson.name || createMut.isPending}>Create</Button>
          </div>
        </Card>
      )}

      {selected.size > 0 && bulkAction === 'merge' && (
        <Card className="mb-6 rounded-[1.75rem] border-white/8 bg-white/5">
          <div className="flex flex-wrap items-end gap-2">
            <Input label="Keep ID" placeholder="Primary person ID" value={mergeKeepId} onChange={(e) => setMergeKeepId(e.target.value)} className="w-40" />
            <Button size="sm" onClick={() => {
              const keepId = parseInt(mergeKeepId);
              if (!keepId) return;
              const mergeIds = selectedIds.filter((id) => id !== keepId);
              bulkMergeMut.mutate({ keepId, mergeIds });
            }} disabled={!mergeKeepId || bulkMergeMut.isPending}>Merge</Button>
            <Button variant="ghost" size="sm" onClick={() => setBulkAction(null)}>Cancel</Button>
          </div>
        </Card>
      )}

      {selected.size > 0 && bulkAction === 'enrich' && (
        <Card className="mb-6 rounded-[1.75rem] border-white/8 bg-white/5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm text-zinc-300">Enrich {selected.size} profiles and apply changes?</span>
            <Button size="sm" onClick={() => bulkEnrichMut.mutate(selectedIds)} disabled={bulkEnrichMut.isPending}>
              {bulkEnrichMut.isPending ? 'Enriching...' : 'Confirm'}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setBulkAction(null)}>Cancel</Button>
          </div>
        </Card>
      )}

      {isLoading ? (
        <Spinner />
      ) : !personList || personList.length === 0 ? (
        <EmptyState message="No people found." />
      ) : (
        <>
          <div className="flex items-center gap-2 mb-3">
            <input
              type="checkbox"
              checked={personList.length > 0 && selected.size === personList.length}
              onChange={toggleSelectAll}
              className="rounded border-zinc-600 bg-zinc-800 text-blue-500"
            />
            <span className="text-xs text-zinc-500">Select page</span>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {personList.map((p) => {
              const photo = parsePhotos(p.photos)[0];
              return (
                <Card key={p.id} className="group overflow-hidden rounded-[2rem] border-white/8 bg-white/5 p-0">
                  <Link to={`/people/${p.id}`} className="block">
                    <div className="relative aspect-[4/5] overflow-hidden">
                      <ProfileTile name={p.name} photo={photo} />
                      <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/30 to-transparent p-4 pt-12">
                        <div className="flex items-end justify-between gap-3">
                          <div className="min-w-0">
                            <h3 className="text-xl font-semibold text-white line-clamp-1">{p.name}</h3>
                            <p className="mt-1 text-xs text-white/70">Updated {formatDate(p.created_at)}</p>
                          </div>
                          <div className="rounded-full bg-white/10 p-2 text-white/70">
                            <ChevronRight size={14} />
                          </div>
                        </div>
                        <div className="mt-3 flex flex-wrap gap-2">
                          {p.nationality && <Badge>{p.nationality}</Badge>}
                          {p.ethnicity && <Badge variant="info">{p.ethnicity}</Badge>}
                          {p.height && <Badge variant="default">{p.height}</Badge>}
                        </div>
                      </div>
                      <div className="absolute left-3 top-3">
                        <input
                          type="checkbox"
                          checked={selected.has(p.id)}
                          onChange={() => toggleSelect(p.id)}
                          className="h-4 w-4 rounded border-white/30 bg-black/30 text-blue-500"
                        />
                      </div>
                    </div>
                  </Link>

                  <div className="p-4">
                    <p className="text-sm text-zinc-300 line-clamp-2">
                      {p.biography || (p.aliases ? `aka ${p.aliases}` : 'No bio available yet.')}
                    </p>

                    <div className="mt-4 flex items-center justify-between gap-2">
                      <Button variant="ghost" size="sm" onClick={() => deleteMut.mutate(p.id)}>
                        <Trash2 size={14} /> Delete
                      </Button>
                      <Button size="sm" onClick={() => toggleSelect(p.id)}>
                        {selected.has(p.id) ? 'Unselect' : 'Select'}
                      </Button>
                    </div>
                  </div>
                </Card>
              );
            })}
          </div>

          <Pagination page={page} hasMore={personList.length === limit} onPrev={prevPage} onNext={nextPage} />
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
