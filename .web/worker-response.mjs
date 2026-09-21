const utf8MediaTypes = new Set([
  'application/javascript',
  'application/x-javascript',
  'text/javascript',
]);

export function normalizePagesResponse(response) {
  const headers = new Headers(response.headers);
  const contentType = headers.get('content-type');
  const mediaType = contentType?.split(';', 1)[0].trim().toLowerCase();

  headers.set('access-control-allow-origin', '*');
  headers.set('x-content-type-options', 'nosniff');
  headers.set('referrer-policy', 'strict-origin-when-cross-origin');

  // The custom domain can retain an HTML shell from the prior deployment while
  // its content-hashed assets have already changed. Do not edge-cache HTML;
  // immutable assets keep the cache policy supplied by the assets binding.
  if (response.status === 404 || mediaType === 'text/html') {
    headers.set('cache-control', 'no-store');
  }

  if (contentType && !/;\s*charset=/i.test(contentType)) {
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
