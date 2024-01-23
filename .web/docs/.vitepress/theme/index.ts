import './tailwind.postcss'
import DefaultTheme from 'vitepress/theme'
import VPButton from 'vitepress/dist/client/theme-default/components/VPButton.vue'
import VPBadge from 'vitepress/dist/client/theme-default/components/VPBadge.vue'
import './styles/vars.css'
import type {Theme} from 'vitepress'
import Layout from "./components/Layout.vue";

export default {
    extends: DefaultTheme,
    Layout: Layout,
    enhanceApp({app}) {
        app.component('VPButton', VPButton)
        app.component('VPBadge', VPBadge)
    }
} satisfies Theme
