import { spawn } from 'node:child_process';

import { buildStatsEnv, fetchCommunityStats } from './community-stats.mjs';

const stats = await fetchCommunityStats();
const statsEnv = buildStatsEnv(stats);

console.log(
  `[community-stats] Discord members: ${statsEnv.GATE_DOCS_DISCORD_MEMBERS}; GitHub stars: ${statsEnv.GATE_DOCS_GITHUB_STARS}`
);

const command = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm';
const child = spawn(command, ['exec', 'vitepress', 'build', 'docs'], {
  env: {
    ...process.env,
    ...statsEnv,
  },
  stdio: 'inherit',
});

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});
