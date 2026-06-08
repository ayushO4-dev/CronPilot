import { api } from './api'

export type Theme = 'dark' | 'light'

const STORAGE_KEY = 'cronpilot-theme'

export function applyTheme(theme: Theme) {
  document.documentElement.setAttribute('data-theme', theme)
  localStorage.setItem(STORAGE_KEY, theme)
}

export function initialTheme(): Theme {
  return localStorage.getItem(STORAGE_KEY) === 'light' ? 'light' : 'dark'
}

// saveTheme applies immediately and best-effort persists server-side.
export async function saveTheme(theme: Theme) {
  applyTheme(theme)
  try {
    await api.put('/api/settings', { theme })
  } catch {
    /* non-fatal: local preference still applied */
  }
}
