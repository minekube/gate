const utf8MediaTypes = new Set([
  'application/javascript',
  'application/x-javascript',
  'text/javascript',
]);

export function normalizePagesResponse(response) {
  const headers = new Headers(response.headers);
  const contentType = headers.get('content-type');

  headers.set('access-control-allow-origin', '*');

  if (response.status === 404) {
    headers.set('cache-control', 'no-store');
  }

  if (contentType && !/;\s*charset=/i.test(contentType)) {
    const mediaType = contentType.split(';', 1)[0].trim().toLowerCase();
    if (mediaType.startsWith('text/') || utf8MediaTypes.has(mediaType)) {
      headers.set('content-type', `${contentType}; charset=utf-8`);
    }
  }

  return new Response(response.body, {
    headers,
    status: response.status,
    statusText: response.statusText,
  });
}
