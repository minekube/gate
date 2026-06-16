export const DEFAULT_COMMUNITY_STATS = {
  discordMembers: 1650,
  githubStars: 1050,
};

export const DEFAULT_DISCORD_INVITE_CODE = 'HvQugYx';
export const DEFAULT_GITHUB_REPO = 'minekube/gate';

const isValidCount = (value) => Number.isInteger(value) && value >= 0;

export function parseGitHubRepoStats(payload) {
  const count = payload?.stargazers_count;
  if (!isValidCount(count)) {
    throw new Error('GitHub API response did not include stargazers_count');
  }
  return count;
}

export function parseDiscordInviteStats(payload) {
  const count = payload?.approximate_member_count;
  if (!isValidCount(count)) {
    throw new Error(
      'Discord invite API response did not include approximate_member_count'
    );
  }
  return count;
}

async function fetchJson(url, { fetchImpl = fetch, headers = {} } = {}) {
  const response = await fetchImpl(url, {
    headers: {
      Accept: 'application/json',
      ...headers,
    },
    signal: AbortSignal.timeout(10_000),
  });

  if (!response.ok) {
    throw new Error(`Request failed with ${response.status} for ${url}`);
  }

  return response.json();
}

export async function fetchGitHubStars({
  fetchImpl,
  repo = process.env.GATE_GITHUB_REPO || DEFAULT_GITHUB_REPO,
} = {}) {
  const payload = await fetchJson(`https://api.github.com/repos/${repo}`, {
    fetchImpl,
    headers: {
      'X-GitHub-Api-Version': '2022-11-28',
      'User-Agent': 'gate-docs-build',
    },
  });
  return parseGitHubRepoStats(payload);
}

export async function fetchDiscordMembers({
  fetchImpl,
  inviteCode = process.env.GATE_DISCORD_INVITE_CODE ||
    DEFAULT_DISCORD_INVITE_CODE,
} = {}) {
  const payload = await fetchJson(
    `https://discord.com/api/v10/invites/${inviteCode}?with_counts=true`,
    { fetchImpl }
  );
  return parseDiscordInviteStats(payload);
}

export async function fetchCommunityStats({ fetchImpl, logger = console } = {}) {
  const stats = {};

  try {
    stats.discordMembers = await fetchDiscordMembers({ fetchImpl });
  } catch (error) {
    logger.warn(
      `[community-stats] Discord member count unavailable, using fallback: ${error.message}`
    );
  }

  try {
    stats.githubStars = await fetchGitHubStars({ fetchImpl });
  } catch (error) {
    logger.warn(
      `[community-stats] GitHub star count unavailable, using fallback: ${error.message}`
    );
  }

  return stats;
}

export function buildStatsEnv(stats = {}) {
  const discordMembers = isValidCount(stats.discordMembers)
    ? stats.discordMembers
    : DEFAULT_COMMUNITY_STATS.discordMembers;
  const githubStars = isValidCount(stats.githubStars)
    ? stats.githubStars
    : DEFAULT_COMMUNITY_STATS.githubStars;

  return {
    GATE_DOCS_DISCORD_MEMBERS: String(discordMembers),
    GATE_DOCS_GITHUB_STARS: String(githubStars),
  };
}
