import pagesWorker from './.worker-dist/index.js';
import { normalizePagesResponse } from './worker-response.mjs';

export default {
  async fetch(request, env, executionContext) {
    const response = await pagesWorker.fetch(request, env, executionContext);
    return normalizePagesResponse(response);
  },
};
