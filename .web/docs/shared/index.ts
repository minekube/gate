import { DefaultTheme } from 'vitepress';

export const discordLink = 'https://minekube.com/discord';
export const gitHubLink = 'https://github.com/minekube';

// Community stats
export const communityStats = {
  discordMembers: 1350,
  githubStars: 900,
};

export const editLink = (project: string): DefaultTheme.EditLink => {
  return {
    pattern: `${gitHubLink}/${project}/edit/master/.web/docs/:path`,
    text: 'Suggest changes to this page',
  };
};

// Simple version info for footer
export const commitRef = 'dev';
export const additionalTitle = '';
