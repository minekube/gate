---
title: "Extending Gate"
linkTitle: "Extend Gate"
weight: 7
description: >
  Instructions on extending Gate with your code.
---

**Gate is build up to be extensible,
let's see how you can plug-in your code!**

_Throughout the docs and code comments we refer to your custom code as `plugin`/`plug-in`._

{{< alert title="Note on Go's plugin system" color="info">}}
We don't support Go's plugin system as it is not a mature solution.
They force your plugin implementation to be highly-coupled with Gate's build toolchain,
the end-result would be very brittle, hard to maintain, and the overhead would
be much higher since the plugin author does not have any control over new versions of Gate.
{{< /alert >}}