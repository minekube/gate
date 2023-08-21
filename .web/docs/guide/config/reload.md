# Auto Config Reload

_Gate watches your file for updates._

---

Gate supports automatic config reloading without restarting the proxy by watching your config file for changes
without disconnecting players.

This is useful for example when you want to change **any setting in the config** like servers, the motd or
switch to Lite mode while staying live.

::: tip
Generally all settings can be changed without disconnecting players,
however some session-related properties like `online-mode` will only apply to newly connected
players that joined after the config update and does not kick players that are already connected with another
online-mode.
:::

## How it works

Gate watches your config file for changes and reloads it automatically when it detects a change.
This is seen as a safe operation, as the config is validated before it is applied.
If it is invalid, the reload is aborted and the proxy continues to run with the last valid config.

## Switching to Lite mode and Connect

If you want to switch to [Lite mode](/guide/lite) or [Connect](/guide/connect), you can do so without restarting the
proxy.
This is useful if you want to test it out or if you want to switch to Lite mode temporarily for maintenance
or migration purposes.

## How to enable it

This feature is always enabled by default, given that you have a config file.

## How to disable it

This can not be disabled.
If you feel like you need to disable it, please [open an issue](
https://github.com/minekube/gate/issues/new?title=Disable%20auto%20config%20reload&body=I%20want%20to%20disable%20auto%20config%20reload%20because%20...).
