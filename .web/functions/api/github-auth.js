// Shared utility for GitHub App authentication
// This replaces PAT (Personal Access Token) authentication with GitHub App authentication

import { createAppAuth } from '@octokit/auth-app';
import { Octokit } from '@octokit/core';

const INSTALLATION_TOKEN_CACHE_KEY = 'github-installation-token';
const INSTALLATION_TOKEN_CACHE_TTL = 50 * 60; // 50 minutes (tokens expire after 60 minutes)

/**
 * Get an authenticated Octokit instance
 * @param {object} env - Cloudflare environment variables
 * @param {object} cache - Cache object (e.g., KV namespace)
 * @returns {Promise<Octokit>} Authenticated Octokit instance
 */
export async function getOctokit(env, cache) {
  // Check cache first
  let token;
  if (cache) {
    const cachedToken = await cache.get(INSTALLATION_TOKEN_CACHE_KEY);
    if (cachedToken) {
      token = cachedToken;
    }
  }

  // Get fresh token if not cached
  if (!token) {
    // Get GitHub App credentials
    const appId = env.GITHUB_APP_ID;
    const privateKey = env.GITHUB_APP_PRIVATE_KEY;
    const installationId = env.GITHUB_APP_INSTALLATION_ID;

    if (!appId || !privateKey || !installationId) {
      throw new Error(
        'GitHub App credentials not configured. Need GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, and GITHUB_APP_INSTALLATION_ID'
      );
    }

    // Create GitHub App auth instance
    const auth = createAppAuth({
      appId,
      privateKey,
    });

    // Get installation access token
    const authResult = await auth({
      type: 'installation',
      installationId,
    });

    token = authResult.token;

    // Cache the token
    if (cache) {
      await cache.put(INSTALLATION_TOKEN_CACHE_KEY, token, {
        expirationTtl: INSTALLATION_TOKEN_CACHE_TTL,
      });
    }
  }

  // Return authenticated Octokit instance
  return new Octokit({
    auth: token,
  });
}
