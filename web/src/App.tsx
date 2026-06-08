import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from './lib/auth'
import { Loading } from './components/ui'
import { Login } from './routes/Login'
import { RequireAuth } from './routes/RequireAuth'
import { Dashboard } from './routes/Dashboard'
import { Overview } from './routes/tabs/Overview'
import { Monitor } from './routes/tabs/Monitor'
import { Terminal } from './routes/tabs/Terminal'
import { Settings } from './routes/tabs/Settings'
import { Services } from './routes/tabs/Services'
import { Placeholder } from './routes/tabs/Placeholder'

export function App() {
  const { loading } = useAuth()
  if (loading) return <Loading text="starting" />

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          path="/"
          element={
            <RequireAuth>
              <Dashboard />
            </RequireAuth>
          }
        >
          <Route index element={<Navigate to="/overview" replace />} />
          <Route path="overview" element={<Overview />} />
          <Route path="monitor" element={<Monitor />} />
          <Route path="services" element={<Services />} />
          <Route path="applications" element={<Placeholder title="Applications" phase={3} />} />
          <Route path="tasks" element={<Placeholder title="Tasks" phase={4} />} />
          <Route path="terminal" element={<Terminal />} />
          <Route path="settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/overview" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
