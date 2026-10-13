// Apply saved theme before paint to avoid a flash (default light).
// External file (not inline) so the CSP can stay script-src 'self'.
try {
  if (localStorage.getItem('sealane-theme') === 'dark') document.documentElement.classList.add('dark')
} catch (e) { /* storage unavailable → stay light */ }
