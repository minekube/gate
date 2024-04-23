import {DefaultTheme} from "vitepress";

export const discordLink = 'https://minekube.com/discord'
export const gitHubLink = 'https://github.com/minekube'

export const editLink = (project: string): DefaultTheme.EditLink => {
    return {
        pattern: `${gitHubLink}/${project}/edit/master/.web/docs/:path`,
        text: 'Suggest changes to this page'
    }
}

// cloudflare envs
export const commitRef = process.env.CF_PAGES_COMMIT_SHA?.slice(0, 8) || 'dev'

export const deployType = (() => {
    if (commitRef === '') {
        return 'local'
    }
    return 'release'
})()

export const additionalTitle = ((): string => {
    if (deployType === 'release') {
        return ''
    }
    return ' (local)'
})()