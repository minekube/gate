import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { createDeploymentConfig } from './deploy-worker.mjs';

const readConfig = async (name) =>
  JSON.parse(await readFile(new URL(`../${name}`, import.meta.url), 'utf8'));

test('production and canary Workers preserve the Pages runtime contract', async () => {
  const production = await readConfig('wrangler.jsonc');
  const canary = await readConfig('wrangler.canary.jsonc');
  const packageJson = JSON.parse(
    await readFile(new URL('../package.json', import.meta.url), 'utf8')
  );
  assert.equal(packageJson.packageManager, 'pnpm@10.11.0');
  assert.equal(packageJson.engines?.node, '>=22.0.0');
  assert.equal(packageJson.scripts['deploy:worker'], 'node scripts/deploy-worker.mjs');
  assert.equal(
    packageJson.scripts['deploy:worker:canary'],
    'node scripts/deploy-worker.mjs --canary'
  );

  const shared = {
    compatibility_date: '2022-11-29',
    main: '.worker-dist/index.js',
    vars: { GITHUB_APP_ID: '2305701' },
    kv_namespaces: [
      {
        binding: 'GITHUB_CACHE',
      },
    ],
    assets: {
      directory: 'docs/.vitepress/dist',
      binding: 'ASSETS',
      not_found_handling: '404-page',
      run_worker_first: ['/api/extensions', '/api/go-modules'],
    },
  };

  for (const config of [production, canary]) {
    for (const [key, value] of Object.entries(shared)) {
      assert.deepEqual(config[key], value);
    }
    assert.equal(config.preview_urls, false);
  }

  assert.equal(production.name, 'gate-minekube-worker');
  assert.equal(production.workers_dev, false);
  assert.deepEqual(production.routes, [
    { pattern: 'gate.minekube.com', custom_domain: true },
  ]);

  assert.equal(canary.name, 'gate-minekube-worker-canary');
  assert.equal(canary.workers_dev, true);
  assert.deepEqual(canary.routes, []);

  const deployment = createDeploymentConfig(
    production,
    'a'.repeat(32),
    process.cwd()
  );
  assert.deepEqual(deployment.kv_namespaces, [
    { binding: 'GITHUB_CACHE', id: 'a'.repeat(32) },
  ]);
  assert.throws(
    () => createDeploymentConfig(production, 'not-a-namespace-id', process.cwd()),
    /GITHUB_CACHE_KV_NAMESPACE_ID/
  );
});

test('the Worker toolchain is reproducible and permits only required install scripts', async () => {
  const packageJson = JSON.parse(
    await readFile(new URL('../package.json', import.meta.url), 'utf8')
  );

  assert.equal(packageJson.devDependencies.wrangler, '4.115.0');
  assert.equal(
    packageJson.devDependencies['@cloudflare/workers-types'],
    '5.20260722.1'
  );
  assert.deepEqual(packageJson.pnpm?.onlyBuiltDependencies, [
    'core-js',
    'esbuild',
    'workerd',
  ]);
});

test('deployment instructions provision the GitHub App private key as a Worker secret', async () => {
  const readme = await readFile(new URL('../README.md', import.meta.url), 'utf8');

  assert.match(readme, /GITHUB_APP_PRIVATE_KEY/);
  assert.match(readme, /wrangler secret put GITHUB_APP_PRIVATE_KEY/);
});
