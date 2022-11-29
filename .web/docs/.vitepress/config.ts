import {defineConfig} from 'vitepress'

import {additionalTitle, commitRef, discordLink, editLink, gitHubLink} from '../shared/'

const ogUrl = 'https://gate.minekube.com'
const ogImage = `${ogUrl}/og-image.png`
const ogTitle = 'Gate Proxy'
const ogDescription = 'Next Generation Minecraft Proxy'

export default defineConfig({
    title: `Gate Proxy${additionalTitle}`,
    description: ogDescription,

    head: [
        ['link', {rel: 'icon', type: 'image/png', href: '/favicon.png'}],
        ['meta', {property: 'og:type', content: 'website'}],
        ['meta', {property: 'og:title', content: ogTitle}],
        ['meta', {property: 'og:image', content: ogImage}],
        ['meta', {property: 'og:url', content: ogUrl}],
        ['meta', {property: 'og:description', content: ogDescription}],
        ['meta', {name: 'theme-color', content: '#646cff'}],
        // [
        //     'script',
        //     {
        //         src: 'https://cdn.usefathom.com/script.js',
        //         'data-site': 'CBDFBSLI',
        //         'data-spa': 'auto',
        //         defer: ''
        //     }
        // ]
    ],

    vue: {
        reactivityTransform: true
    },

    themeConfig: {
        logo: '/minekube-logo.png',

        editLink: editLink('gate'),

        socialLinks: [
            {icon: 'discord', link: discordLink},
            {icon: 'github', link: `${gitHubLink}/gate`}
        ],

        // algolia: {
        //     appId: '7H67QR5P0A',
        //     apiKey: 'deaab78bcdfe96b599497d25acc6460e',
        //     indexName: 'vitejs',
        //     searchParameters: {
        //         facetFilters: ['tags:en']
        //     }
        // },

        // carbonAds: {
        //     code: 'CEBIEK3N',
        //     placement: 'vitejsdev'
        // },

        footer: {
            message: `Released under the MIT License. (web version: ${commitRef})`,
            copyright: 'Copyright © 2022-present Minekube & Contributors'
        },

        nav: [
            {text: 'Quick Start', link: '/guide/quick-start'},
            {text: 'Guide', link: '/guide/'},
            {text: 'Developers Guide', link: '/developers/'},
            {text: 'Config', link: '/guide/config/'},
            {text: 'Downloads', link: '/guide/install/'},
            {
                text: 'Connect',
                link: 'https://connect.minekube.com',
            },
        ],

        sidebar: {
            '/guide/': [
                {
                    text: 'Getting Started',
                    items: [
                        {text: 'Introduction', link: '/guide/'},
                        {text: 'Quick Start', link: '/guide/quick-start'},
                        {text: 'Why', link: '/guide/why'},
                    ]
                },
                {
                    text: 'Installation',
                    items: [
                        {
                            text: 'Prebuilt Binaries',
                            link: '/guide/install/binaries'
                        },
                        {
                            text: 'Go Install',
                            link: '/guide/install/go'
                        },
                        {
                            text: 'Docker',
                            link: '/guide/install/docker'
                        },
                        {
                            text: 'Kubernetes',
                            link: '/guide/install/kubernetes'
                        },
                    ]
                },
                {
                    text: 'Guide',
                    items: [
                        {
                            text: 'Configurations',
                            link: '/guide/config/',
                        },
                        {
                            text: 'Enabling Connect',
                            link: '/guide/connect'
                        },
                        {
                            text: 'Builtin Commands',
                            link: '/guide/builtin-commands'
                        },
                        {
                            text: 'Compatibility',
                            link: '/guide/compatibility'
                        },
                        {
                            text: 'Rate Limiting',
                            link: '/guide/rate-limiting'
                        },
                    ]
                },
            ],
            '/developers/': [
                {
                    text: 'Developers Guide',
                    items: [
                        {
                            text: 'Introduction',
                            link: '/guide/developers/'
                        },
                    ]
                },
            ],
            // '/config/': [
            //     {
            //         text: 'Configuration',
            //         items: [
            //             {text: 'Backend Servers', link: '/config/servers'},
            //         ]
            //     },
            // ],
        }
    }
})
