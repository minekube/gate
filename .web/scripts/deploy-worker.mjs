import { spawn } from 'node:child_process';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const projectRoot = path.resolve(fileURLToPath(new URL('..', import.meta.url)));
const namespaceIdPattern = /^[0-9a-f]{32}$/i;

export function createDeploymentConfig(config, namespaceId, rootDirectory) {
  if (!namespaceIdPattern.test(namespaceId ?? '')) {
    throw new Error(
      'GITHUB_CACHE_KV_NAMESPACE_ID must be a 32-character hexadecimal Workers KV namespace ID'
    );
  }

  if (
    config.kv_namespaces?.length !== 1 ||
    config.kv_namespaces[0]?.binding !== 'GITHUB_CACHE'
  ) {
    throw new Error('Wrangler config must define exactly one GITHUB_CACHE KV namespace');
  }

  return {
    ...config,
    main: path.resolve(rootDirectory, config.main),
    kv_namespaces: [
      { ...config.kv_namespaces[0], id: namespaceId },
    ],
    assets: {
      ...config.assets,
      directory: path.resolve(rootDirectory, config.assets.directory),
    },
  };
}

async function deploy(configName) {
  const config = JSON.parse(
    await readFile(path.join(projectRoot, configName), 'utf8')
  );
  const deploymentConfig = createDeploymentConfig(
    config,
    process.env.GITHUB_CACHE_KV_NAMESPACE_ID,
    projectRoot
  );
  const generatedConfigPath = path.join(
    projectRoot,
    '.wrangler',
    'worker-configs',
    configName
  );

  await mkdir(path.dirname(generatedConfigPath), { recursive: true });
  await writeFile(generatedConfigPath, `${JSON.stringify(deploymentConfig, null, 2)}\n`);

  await new Promise((resolve, reject) => {
    const command = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm';
    const child = spawn(
      command,
      ['exec', 'wrangler', 'deploy', '--config', generatedConfigPath],
      { cwd: projectRoot, stdio: 'inherit' }
    );
    child.on('error', reject);
    child.on('exit', (code, signal) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`wrangler deploy exited with ${signal ?? `code ${code}`}`));
    });
  });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const configName = process.argv[2] === '--canary'
    ? 'wrangler.canary.jsonc'
    : process.argv.length === 2
      ? 'wrangler.jsonc'
      : null;

  if (!configName) {
    console.error('Usage: node scripts/deploy-worker.mjs [--canary]');
    process.exitCode = 1;
  } else {
    try {
      await deploy(configName);
    } catch (error) {
      console.error(error.message);
      process.exitCode = 1;
    }
  }
}
