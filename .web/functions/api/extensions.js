// functions/api/extensions.js

const CACHE_DURATION = 60 * 60; // Cache duration in seconds

export async function onRequest(context) {
  const githubApiUrl = 'https://api.github.com/search/repositories?q=topic:gate-extension&sort=stars&order=desc';
  const cacheKey = 'gate-extension-repositories';

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
    const libraries = data.items.map((item) => ({
      name: item.name,
      owner: item.owner.login,
      description: item.description,
      stars: item.stargazers_count,
      url: item.html_url,
    }));

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
