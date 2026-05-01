// Configure HTMX to swap 4xx/5xx responses when the server provides HTML
// (e.g. an error partial). Plain-text error bodies are filtered out in
// htmx:beforeSwap so they don't clobber the form, and surface as a toast
// instead.
htmx.config.responseHandling = [
  { code: '204', swap: false },
  { code: '[23]..', swap: true },
  { code: '[45]..', swap: true, error: true },
  { code: '...', swap: false, error: true },
];

document.addEventListener('htmx:beforeSwap', (event) => {
  const xhr = event.detail.xhr;
  if (!xhr || xhr.status < 400) return;
  const ct = xhr.getResponseHeader('Content-Type') || '';
  if (!ct.includes('text/html')) {
    event.detail.shouldSwap = false;
  }
});

document.addEventListener('htmx:sendError', () => {
  showToast('Network error. Please check your connection and try again.');
});

document.addEventListener('htmx:responseError', (event) => {
  const xhr = event.detail.xhr;
  const ct = xhr.getResponseHeader('Content-Type') || '';
  // If the response body was HTML, the swap already surfaced the error inline;
  // skip the toast to avoid duplicate UI.
  if (ct.includes('text/html')) return;

  const body = (xhr.responseText || '').trim();
  const message = body && body.length < 200
    ? body
    : `Request failed (${xhr.status}).`;
  showToast(message);
});

function showToast(message) {
  const container = document.getElementById('toast-container');
  if (!container) return;
  const toast = document.createElement('div');
  toast.className = 'toast toast-error';
  toast.setAttribute('role', 'alert');
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => {
    toast.classList.add('toast-leaving');
    toast.addEventListener('animationend', () => toast.remove(), { once: true });
  }, 5000);
}
