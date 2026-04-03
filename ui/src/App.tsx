
import { Sidebar } from './components/ui/Sidebar';
import { DashboardPage } from './pages/DashboardPage';
import { ToastContainer } from './components/ui/Toast';
import { useWebSocket } from './hooks/useWebSocket';
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts';

function App() {
  // Initialize live data feed (mock or real WebSocket)
  useWebSocket();
  // Global keyboard shortcuts
  useKeyboardShortcuts();

  return (
    <div className="flex h-full overflow-hidden bg-surface-900">
      <Sidebar />
      <DashboardPage />
      <ToastContainer />
    </div>
  );
}

export default App;
