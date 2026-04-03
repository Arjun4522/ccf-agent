import { useEffect } from 'react';
import { useStore } from '../store';

export function useKeyboardShortcuts() {
  const toggleStreamPause = useStore(s => s.toggleStreamPause);
  const setActivePage = useStore(s => s.setActivePage);
  const clearDetections = useStore(s => s.clearDetections);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      // Ignore if typing in input/textarea
      const tag = (e.target as HTMLElement).tagName.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return;

      if (e.code === 'Space' && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
        e.preventDefault();
        toggleStreamPause();
      }

      if (e.key === '1') setActivePage('dashboard');
      if (e.key === '2') setActivePage('detections');
      if (e.key === '3') setActivePage('analytics');
      if (e.key === '4') setActivePage('response');
      if (e.key === '5') setActivePage('quarantine');
      if (e.key === '6') setActivePage('field');
      if (e.key === '7') setActivePage('settings');

      if (e.ctrlKey && e.key === 'l') {
        e.preventDefault();
        clearDetections();
      }
    };

    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [toggleStreamPause, setActivePage, clearDetections]);
}
