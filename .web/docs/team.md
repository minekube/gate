---
title: "Gate Development Team - Meet the Creators"
description: "Meet the international team behind Gate Minecraft proxy development. Learn about the developers, contributors, and maintainers building the next-generation Minecraft proxy."
---

<script setup>
import {
  VPTeamPage,
  VPTeamPageTitle,
  VPTeamPageSection,
  VPTeamMembers
} from 'vitepress/theme';
import { core, emeriti } from './_data/team'
</script>

<VPTeamPage>
  <VPTeamPageTitle>
    <template #title>Meet the Team</template>
    <template #lead>
      The development of Minekube OSS & services is guided by international contributors and a core team, some of whom
      have chosen to be featured below.
    </template>
  </VPTeamPageTitle>
  <VPTeamMembers :members="core" />
</VPTeamPage>
