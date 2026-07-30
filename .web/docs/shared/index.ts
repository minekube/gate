import { DefaultTheme } from 'vitepress';

export const discordLink = 'https://minekube.com/discord';
export const gitHubLink = 'https://github.com/minekube';

// Community stats
declare const __COMMUNITY_STATS__:
  | {
      discordMembers: number;
      githubStars: number;
    }
  | undefined;

export const communityStats = {
  discordMembers:
    typeof __COMMUNITY_STATS__ !== 'undefined'
      ? __COMMUNITY_STATS__.discordMembers
      : 1650,
  githubStars:
    typeof __COMMUNITY_STATS__ !== 'undefined'
      ? __COMMUNITY_STATS__.githubStars
      : 1050,
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
