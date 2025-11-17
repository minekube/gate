/// <reference types="@cloudflare/workers-types" />

// functions/api/extensions.ts

import { getOctokit, type GitHubAppEnv } from './github-auth';

const CACHE_DURATION = 60 * 60; // Cache duration in seconds

interface ExtensionRepository {
  name: string;
  owner: string;
  description: string | null;
  stars: number;
  url: string;
}

interface CloudflarePagesFunctionEnv extends GitHubAppEnv {
  GITHUB_CACHE: KVNamespace;
}

interface CloudflarePagesFunctionContext {
  env: CloudflarePagesFunctionEnv;
}

export async function onRequest(
  context: CloudflarePagesFunctionContext
): Promise<Response> {
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

    const libraries: ExtensionRepository[] = data.items.map((item) => ({
      name: item.name,
      owner: item.owner?.login ?? 'unknown',
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
    const errorMessage =
      error instanceof Error ? error.message : 'Unknown error';
    return new Response(`Error fetching data: ${errorMessage}`, {
      status: 500,
    });
  }
}

