/// <reference types="@cloudflare/workers-types" />

// Shared utility for GitHub App authentication
// This replaces PAT (Personal Access Token) authentication with GitHub App authentication
// Uses JWT tokens for public repository searches (no installation required)

import { createAppAuth } from '@octokit/auth-app';
import { Octokit } from '@octokit/core';

const JWT_TOKEN_CACHE_KEY = 'github-jwt-token';
const JWT_TOKEN_CACHE_TTL = 8 * 60; // 8 minutes (JWT tokens expire after 10 minutes)

/**
 * Environment variables interface for GitHub App configuration
 */
export interface GitHubAppEnv {
  GITHUB_APP_ID: string;
  GITHUB_APP_PRIVATE_KEY: string;
  GITHUB_CACHE?: KVNamespace;
}

/**
 * Get an authenticated Octokit instance using GitHub App JWT
 * @param env - Cloudflare environment variables
 * @param cache - Cache object (e.g., KV namespace)
 * @returns Authenticated Octokit instance
 */
export async function getOctokit(
  env: GitHubAppEnv,
  cache?: KVNamespace
): Promise<Octokit> {
  // Get GitHub App credentials
  const appId = env.GITHUB_APP_ID;
  const privateKey = env.GITHUB_APP_PRIVATE_KEY;

  if (!appId || !privateKey) {
    throw new Error(
      'GitHub App credentials not configured. Need GITHUB_APP_ID and GITHUB_APP_PRIVATE_KEY'
    );
  }

  // Check cache first for JWT token
  let token: string | null = null;
  if (cache) {
    const cachedToken = await cache.get(JWT_TOKEN_CACHE_KEY);
    if (cachedToken) {
      token = cachedToken;
    }
  }

  // Generate fresh JWT token if not cached
  if (!token) {
    // Create GitHub App auth instance
    const auth = createAppAuth({
      appId,
      privateKey,
    });

    // Get JWT token (no installation needed for public repo searches)
    const authResult = await auth({
      type: 'app',
    });

    token = authResult.token;

    // Cache the token (JWT tokens are valid for 10 minutes)
    if (cache && token) {
      await cache.put(JWT_TOKEN_CACHE_KEY, token, {
        expirationTtl: JWT_TOKEN_CACHE_TTL,
      });
    }
  }

  // Return authenticated Octokit instance
  return new Octokit({
    auth: token,
  });
}

