import { Routes, Route, Navigate } from 'react-router-dom';
import { Layout } from '@/components/Layout';
import { DashboardPage } from '@/pages/DashboardPage';
import { SourcesPage } from '@/pages/SourcesPage';
import { GalleriesPage } from '@/pages/GalleriesPage';
import { GalleryDetailPage } from '@/pages/GalleryDetailPage';
import { ImagesPage } from '@/pages/ImagesPage';
import { VideosPage } from '@/pages/VideosPage';
import { PeoplePage } from '@/pages/PeoplePage';
import { PersonDetailPage } from '@/pages/PersonDetailPage';
import { AdminPage } from '@/pages/AdminPage';

export function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Navigate to="/dashboard" replace />} />
        <Route path="dashboard" element={<DashboardPage />} />
        <Route path="sources" element={<SourcesPage />} />
        <Route path="galleries" element={<GalleriesPage />} />
        <Route path="galleries/:id" element={<GalleryDetailPage />} />
        <Route path="images" element={<ImagesPage />} />
        <Route path="videos" element={<VideosPage />} />
        <Route path="people" element={<PeoplePage />} />
        <Route path="people/:id" element={<PersonDetailPage />} />
        <Route path="admin" element={<AdminPage />} />
      </Route>
    </Routes>
  );
}
