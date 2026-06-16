import assert from 'node:assert/strict';
import test from 'node:test';

import {
  DEFAULT_COMMUNITY_STATS,
  buildStatsEnv,
  parseDiscordInviteStats,
  parseGitHubRepoStats,
} from './community-stats.mjs';

test('parseGitHubRepoStats reads stargazers_count', () => {
  assert.equal(parseGitHubRepoStats({ stargazers_count: 1050 }), 1050);
});

test('parseDiscordInviteStats reads approximate_member_count', () => {
  assert.equal(parseDiscordInviteStats({ approximate_member_count: 1652 }), 1652);
});

test('stats parsers reject missing or invalid values', () => {
  assert.throws(() => parseGitHubRepoStats({ stargazers_count: -1 }));
  assert.throws(() => parseDiscordInviteStats({ approximate_member_count: '1652' }));
});

test('buildStatsEnv preserves fetched counts and falls back per field', () => {
  assert.deepEqual(
    buildStatsEnv({ discordMembers: 1652, githubStars: undefined }),
    {
      GATE_DOCS_DISCORD_MEMBERS: '1652',
      GATE_DOCS_GITHUB_STARS: String(DEFAULT_COMMUNITY_STATS.githubStars),
    }
  );
});
