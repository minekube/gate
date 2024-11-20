import { defineConfig } from 'vitepress';

import {
  additionalTitle,
  commitRef,
  discordLink,
  editLink,
  gitHubLink,
} from '../shared/';

const ogUrl = 'https://gate.minekube.com';
const ogImage = `${ogUrl}/og-image.png`;
const ogTitle = 'Gate Proxy';
const ogDescription = 'Next Generation Minecraft Proxy';

export default defineConfig({
  title: `Gate Proxy${additionalTitle}`,
  description: ogDescription,
  appearance: 'dark',

  sitemap: {
    hostname: ogUrl,
  },

  head: [
    ['link', { rel: 'icon', type: 'image/png', href: '/favicon.png' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: ogTitle }],
    ['meta', { property: 'og:image', content: ogImage }],
    ['meta', { property: 'og:url', content: ogUrl }],
    ['meta', { property: 'og:description', content: ogDescription }],
    ['meta', { name: 'theme-color', content: '#646cff' }],
    // [
    //     'script',
    //     {
    //         src: 'https://cdn.usefathom.com/script.js',
    //         'data-site': 'CBDFBSLI',
    //         'data-spa': 'auto',
    //         defer: ''
    //     }
    // ]
    [
      'script',
      {},
      `!function(t,e){var o,n,p,r;e.__SV||(window.posthog=e,e._i=[],e.init=function(i,s,a){function g(t,e){var o=e.split(".");2==o.length&&(t=t[o[0]],e=o[1]),t[e]=function(){t.push([e].concat(Array.prototype.slice.call(arguments,0)))}}(p=t.createElement("script")).type="text/javascript",p.async=!0,p.src=s.api_host+"/static/array.js",(r=t.getElementsByTagName("script")[0]).parentNode.insertBefore(p,r);var u=e;for(void 0!==a?u=e[a]=[]:a="posthog",u.people=u.people||[],u.toString=function(t){var e="posthog";return"posthog"!==a&&(e+="."+a),t||(e+=" (stub)"),e},u.people.toString=function(){return u.toString(1)+".people (stub)"},o="capture identify alias people.set people.set_once set_config register register_once unregister opt_out_capturing has_opted_out_capturing opt_in_capturing reset isFeatureEnabled onFeatureFlags getFeatureFlag getFeatureFlagPayload reloadFeatureFlags group updateEarlyAccessFeatureEnrollment getEarlyAccessFeatures getActiveMatchingSurveys getSurveys onSessionId".split(" "),n=0;n<o.length;n++)g(u,o[n]);e._i.push([i,s,a])},e.__SV=1)}(document,window.posthog||[]);
            posthog.init('phc_h17apkvCV2aUlSQA4BB7WP7AmZHaU14AKnAe9f3ij5S',{api_host:'https://ph.minekube.com'})`,
    ],
  ],

  vue: {
    reactivityTransform: true,
  },

  themeConfig: {
    logo: '/minekube-logo.png',

    editLink: editLink('gate'),

    socialLinks: [
      { icon: 'discord', link: discordLink },
      { icon: 'github', link: `${gitHubLink}/gate` },
    ],

    search: {
      provider: 'algolia',
      options: {
        appId: 'CUJMPRQVZJ',
        apiKey: 'f3a1d3d48a15f78e39d6401b86318ed7',
        indexName: 'gate-minekube',
      },
    },

    // carbonAds: {
    //     code: 'CEBIEK3N',
    //     placement: 'vitejsdev'
    // },

    footer: {
      message: `Released under the MIT License. (web version: ${commitRef})`,
      copyright: 'Copyright © 2022-present Minekube & Contributors',
    },

    nav: [
      { text: 'Guide', link: '/guide/' },
      { text: 'Lite mode', link: '/guide/lite' },
      { text: 'Developers', link: '/developers/' },
      { text: 'API', link: '/developers/api/' },
      { text: 'Config', link: '/guide/config/' },
      { text: 'Downloads', link: '/guide/install/' },
      { text: 'Extensions', link: '/extensions' },
      {
        text: 'Blog',
        link: 'https://connect.minekube.com/blog',
      },
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
            { text: 'Introduction', link: '/guide/' },
            { text: 'Quick Start', link: '/guide/quick-start' },
            { text: 'Why', link: '/guide/why' },
          ],
        },
        {
          text: 'Installation',
          items: [
            {
              text: 'Prebuilt Binaries',
              link: '/guide/install/binaries',
            },
            {
              text: 'Go Install',
              link: '/guide/install/go',
            },
            {
              text: 'Docker',
              link: '/guide/install/docker',
            },
            {
              text: 'Kubernetes',
              link: '/guide/install/kubernetes',
            },
          ],
        },
        {
          text: 'Guides',
          items: [
            {
              text: 'Lite mode',
              link: '/guide/lite',
            },
            {
              text: 'Developers Guide',
              link: '/developers/',
            },
            {
              text: 'HTTP API',
              link: '/developers/api/',
            },
            {
              text: 'Compatibility',
              link: '/guide/compatibility',
            },
          ],
        },
        {
          text: 'Configuration',
          items: [
            {
              text: 'Enabling Connect',
              link: '/guide/connect',
            },
            {
              text: 'Complete Configuration',
              link: '/guide/config/',
            },
            {
              text: 'Auto Reload',
              link: '/guide/config/reload',
            },
            {
              text: 'Builtin Commands',
              link: '/guide/builtin-commands',
            },
            {
              text: 'Rate Limiting',
              link: '/guide/rate-limiting',
            },
          ],
        },
        {
          text: 'Security',
          items: [
            {
              text: 'Cybersecurity',
              link: '/guide/security/',
            },
            {
              text: 'DDoS Protection',
              link: '/guide/security/ddos',
            },
          ],
        },
      ],
      '/developers/': [
        {
          text: 'Developers Guide',
          items: [
            {
              text: 'Introduction',
              link: '/developers/',
            },
            {
              text: 'Events',
              link: '/developers/events',
            },
          ],
        },
        {
          text: 'HTTP API',
          link: '/developers/api/',
          items: [
            {
              text: 'Getting Started',
              link: '/developers/api/',
            },
            {
              text: 'Definition',
              link: '/developers/api/definition',
            },
            {
              text: 'TypeScript',
              link: '/developers/api/typescript/',
              items: [
                {
                  text: 'Bun',
                  link: '/developers/api/typescript/bun/',
                },
                {
                  text: 'Node.js',
                  link: '/developers/api/typescript/node/',
                },
                {
                  text: 'Web',
                  link: '/developers/api/typescript/web/',
                },
              ],
            },
            {
              text: 'Python',
              link: '/developers/api/python/',
            },
            {
              text: 'Go',
              link: '/developers/api/go/',
            },
            {
              text: 'Rust',
              link: '/developers/api/rust/',
            },
            {
              text: 'Kotlin',
              link: '/developers/api/kotlin/',
            },
            {
              text: 'Java',
              link: '/developers/api/java/',
            },
            {
              text: 'Glossary',
              link: '/developers/api/glossary',
            },
          ],
        },
        {
          text: 'Learn by Examples',
          items: [
            {
              text: 'Simple Proxy',
              link: '/developers/examples/simple-proxy',
            },
          ],
        },
        {
          text: 'Back to Guides',
          link: '/guide/',
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
    },
  },
});
