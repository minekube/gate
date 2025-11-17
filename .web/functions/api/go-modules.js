import { getOctokit } from './github-auth.js';

const CACHE_DURATION = 60 * 60; // Cache duration in seconds

export async function onRequest(context) {
  const cacheKey = 'go-module-repositories';

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

    // Search for code files with go.mod containing go.minekube.com
    const { data } = await octokit.request('GET /search/code', {
      q: 'filename:go.mod go.minekube.com in:file',
      sort: 'indexed',
      order: 'desc',
    });

    const uniqueRepos = new Set(); // Set to track processed repository names
    const libraries = [];

    for (const item of data.items) {
      const repo = item.repository;

      // Skip duplicate repositories
      if (uniqueRepos.has(repo.full_name)) {
        console.log(`Skipping duplicate repository: ${repo.full_name}`);
        continue;
      }

      // Mark the repository as processed
      uniqueRepos.add(repo.full_name);

      // Fetch additional repo details using Octokit
      try {
        const [owner, repoName] = repo.full_name.split('/');
        const { data: repoDetails } = await octokit.request(
          'GET /repos/{owner}/{repo}',
          {
            owner,
            repo: repoName,
          }
        );

        libraries.push({
          name: repo.name,
          owner: repo.owner.login,
          description: repoDetails.description || 'No description',
          stars: repoDetails.stargazers_count,
          url: repo.html_url,
        });
      } catch (error) {
        console.error(
          `Error fetching repository details for ${repo.full_name}: ${error.message}`
        );
        // Continue with next repository
      }
    }

    // Cache the deduplicated response
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
