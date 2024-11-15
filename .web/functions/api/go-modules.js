// functions/api/go-modules.js

const CACHE_DURATION = 60 * 60; // Cache duration in seconds

export async function onRequest(context) {
  const githubApiUrl = 'https://api.github.com/search/code?q=filename:go.mod+go.minekube.com+in:file&sort=indexed&order=desc';
  const cacheKey = 'go-module-repositories';

  // Access the KV namespace and GitHub token from context.env
  const GITHUB_CACHE = context.env.GITHUB_CACHE;
  const githubToken = context.env.GITHUB_TOKEN;

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
    // Fetch data from GitHub API
    const response = await fetch(githubApiUrl, {
      headers: {
        'Accept': 'application/vnd.github.v3+json',
        'Authorization': `token ${githubToken}`,
        'User-Agent': 'CloudflarePagesGateExtension/1.0 (+https://developers.cloudflare.com/pages)',
      },
    });

    if (!response.ok) {
      return new Response('Error fetching data from GitHub API', { status: 500 });
    }

    const data = await response.json();
    const libraries = [];

    for (const item of data.items) {
      const repo = item.repository;

      // Fetch additional repo details
      const repoDetails = await fetchRepositoryDetails(repo.full_name, githubToken);
      if (repoDetails) {
        libraries.push({
          name: repo.name,
          owner: repo.owner.login,
          description: repoDetails.description || 'No description',
          stars: repoDetails.stargazers_count,
          url: repo.html_url,
        });
      }
    }

    // Cache the response
    await GITHUB_CACHE.put(cacheKey, JSON.stringify(libraries), { expirationTtl: CACHE_DURATION });

    return new Response(JSON.stringify(libraries), {
      headers: {
        'Content-Type': 'application/json',
        'Access-Control-Allow-Origin': '*',
        'Access-Control-Allow-Methods': 'GET',
        'Access-Control-Allow-Headers': 'Content-Type',
      },
    });
  } catch (error) {
    return new Response(`Error fetching data: ${error}`, { status: 500 });
  }
}

// Helper function to fetch repository details
async function fetchRepositoryDetails(repoFullName, githubToken) {
  const repoUrl = `https://api.github.com/repos/${repoFullName}`;

  const response = await fetch(repoUrl, {
    headers: {
      'Accept': 'application/vnd.github.v3+json',
      'Authorization': `token ${githubToken}`,
      'User-Agent': 'CloudflarePagesGateExtension/1.0 (+https://developers.cloudflare.com/pages)',
    },
  });

  if (!response.ok) {
    console.error(`Error fetching repository details: ${response.status}`);
    return null;
  }

  return await response.json();
}
