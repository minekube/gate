import Theme from 'vitepress/theme'
import VPButton from 'vitepress/dist/client/theme-default/components/VPButton.vue'
import VPBadge from 'vitepress/dist/client/theme-default/components/VPBadge.vue'
import './styles/vars.css'

export default {
  ...Theme,
  enhanceApp({ app }) {
    app.component('VPButton', VPButton)
    app.component('VPBadge', VPBadge)
  }
}
