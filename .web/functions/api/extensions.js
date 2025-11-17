// functions/api/extensions.js

import { getOctokit } from './github-auth.js';

const CACHE_DURATION = 60 * 60; // Cache duration in seconds

export async function onRequest(context) {
  const cacheKey = 'gate-extension-repositories';

  // Access the KV namespace from context.env
  const GITHUB_CACHE = context.env.GITHUB_CACHE;

  // Check for cached data
  const cachedResponse = await GITHUB_CACHE.get(cacheKey);
  if (cachedResponse) {
    return new Response(cachedResponse, {
      headers: {
        'Content-Type': 'application/json',
        'Access-Control-Allow-Origin': '*',
      },
    });
  }

  try {
    // Get authenticated Octokit instance
    const octokit = await getOctokit(context.env, GITHUB_CACHE);

    // Search for repositories with topic:gate-extension
    const { data } = await octokit.request('GET /search/repositories', {
      q: 'topic:gate-extension',
      sort: 'stars',
      order: 'desc',
    });

    const libraries = data.items.map((item) => ({
      name: item.name,
      owner: item.owner.login,
      description: item.description,
      stars: item.stargazers_count,
      url: item.html_url,
    }));

    // Cache the response
    await GITHUB_CACHE.put(cacheKey, JSON.stringify(libraries), {
      expirationTtl: CACHE_DURATION,
    });

    return new Response(JSON.stringify(libraries), {
      headers: {
        'Content-Type': 'application/json',
        'Access-Control-Allow-Origin': '*',
        'Access-Control-Allow-Methods': 'GET',
        'Access-Control-Allow-Headers': 'Content-Type',
      },
    });
  } catch (error) {
    return new Response(`Error fetching data: ${error.message}`, {
      status: 500,
    });
  }
}
