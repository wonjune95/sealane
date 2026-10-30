import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from './App'
import { SESSION_EVENT } from './api/client'
import { ToastProvider } from './components/Toast'
import { NamespaceProvider } from './ui/namespace'
import { applyTheme, initialTheme } from './ui/theme'
import { locale } from './i18n'
import './index.css'

applyTheme(initialTheme()) // set <html> class before first paint
document.documentElement.lang = locale

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
})
window.addEventListener(SESSION_EVENT, () => void queryClient.invalidateQueries({ queryKey: ['capabilities'] }))

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <NamespaceProvider>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </NamespaceProvider>
      </ToastProvider>
    </QueryClientProvider>
  </React.StrictMode>,
)
