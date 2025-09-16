---
title: "Gate Protocol Documentation - Generated API Reference"
description: "Generated protocol documentation for Gate Minecraft proxy API. Complete reference for all API endpoints, messages, and data structures."
---

# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [minekube/gate/v1/gate_service.proto](#minekube_gate_v1_gate_service-proto)
    - [APIConfig](#minekube-gate-v1-APIConfig)
    - [AddLiteRouteBackendRequest](#minekube-gate-v1-AddLiteRouteBackendRequest)
    - [AddLiteRouteBackendResponse](#minekube-gate-v1-AddLiteRouteBackendResponse)
    - [ApplyConfigRequest](#minekube-gate-v1-ApplyConfigRequest)
    - [ApplyConfigResponse](#minekube-gate-v1-ApplyConfigResponse)
    - [BedrockPlayerData](#minekube-gate-v1-BedrockPlayerData)
    - [ConnectConfig](#minekube-gate-v1-ConnectConfig)
    - [ConnectPlayerRequest](#minekube-gate-v1-ConnectPlayerRequest)
    - [ConnectPlayerResponse](#minekube-gate-v1-ConnectPlayerResponse)
    - [DisconnectPlayerRequest](#minekube-gate-v1-DisconnectPlayerRequest)
    - [DisconnectPlayerResponse](#minekube-gate-v1-DisconnectPlayerResponse)
    - [ForwardingConfig](#minekube-gate-v1-ForwardingConfig)
    - [GateConfig](#minekube-gate-v1-GateConfig)
    - [GetConfigRequest](#minekube-gate-v1-GetConfigRequest)
    - [GetConfigResponse](#minekube-gate-v1-GetConfigResponse)
    - [GetLiteRouteRequest](#minekube-gate-v1-GetLiteRouteRequest)
    - [GetLiteRouteResponse](#minekube-gate-v1-GetLiteRouteResponse)
    - [GetPlayerRequest](#minekube-gate-v1-GetPlayerRequest)
    - [GetPlayerResponse](#minekube-gate-v1-GetPlayerResponse)
    - [GetStatusRequest](#minekube-gate-v1-GetStatusRequest)
    - [GetStatusResponse](#minekube-gate-v1-GetStatusResponse)
    - [HealthServiceConfig](#minekube-gate-v1-HealthServiceConfig)
    - [JavaConfig](#minekube-gate-v1-JavaConfig)
    - [JavaConfig.ForcedHostsEntry](#minekube-gate-v1-JavaConfig-ForcedHostsEntry)
    - [JavaConfig.ServersEntry](#minekube-gate-v1-JavaConfig-ServersEntry)
    - [ListLiteRoutesRequest](#minekube-gate-v1-ListLiteRoutesRequest)
    - [ListLiteRoutesResponse](#minekube-gate-v1-ListLiteRoutesResponse)
    - [ListPlayersRequest](#minekube-gate-v1-ListPlayersRequest)
    - [ListPlayersResponse](#minekube-gate-v1-ListPlayersResponse)
    - [ListServersRequest](#minekube-gate-v1-ListServersRequest)
    - [ListServersResponse](#minekube-gate-v1-ListServersResponse)
    - [LiteConfig](#minekube-gate-v1-LiteConfig)
    - [LiteRoute](#minekube-gate-v1-LiteRoute)
    - [LiteRouteBackend](#minekube-gate-v1-LiteRouteBackend)
    - [LiteRouteFallback](#minekube-gate-v1-LiteRouteFallback)
    - [LiteRouteFallbackPlayers](#minekube-gate-v1-LiteRouteFallbackPlayers)
    - [LiteRouteFallbackVersion](#minekube-gate-v1-LiteRouteFallbackVersion)
    - [LiteRouteOptions](#minekube-gate-v1-LiteRouteOptions)
    - [Player](#minekube-gate-v1-Player)
    - [RegisterServerRequest](#minekube-gate-v1-RegisterServerRequest)
    - [RegisterServerResponse](#minekube-gate-v1-RegisterServerResponse)
    - [RemoveLiteRouteBackendRequest](#minekube-gate-v1-RemoveLiteRouteBackendRequest)
    - [RemoveLiteRouteBackendResponse](#minekube-gate-v1-RemoveLiteRouteBackendResponse)
    - [RequestCookieRequest](#minekube-gate-v1-RequestCookieRequest)
    - [RequestCookieResponse](#minekube-gate-v1-RequestCookieResponse)
    - [Server](#minekube-gate-v1-Server)
    - [StatusConfig](#minekube-gate-v1-StatusConfig)
    - [StoreCookieRequest](#minekube-gate-v1-StoreCookieRequest)
    - [StoreCookieResponse](#minekube-gate-v1-StoreCookieResponse)
    - [StringList](#minekube-gate-v1-StringList)
    - [UnregisterServerRequest](#minekube-gate-v1-UnregisterServerRequest)
    - [UnregisterServerResponse](#minekube-gate-v1-UnregisterServerResponse)
    - [UpdateLiteRouteFallbackRequest](#minekube-gate-v1-UpdateLiteRouteFallbackRequest)
    - [UpdateLiteRouteFallbackResponse](#minekube-gate-v1-UpdateLiteRouteFallbackResponse)
    - [UpdateLiteRouteOptionsRequest](#minekube-gate-v1-UpdateLiteRouteOptionsRequest)
    - [UpdateLiteRouteOptionsResponse](#minekube-gate-v1-UpdateLiteRouteOptionsResponse)
    - [UpdateLiteRouteStrategyRequest](#minekube-gate-v1-UpdateLiteRouteStrategyRequest)
    - [UpdateLiteRouteStrategyResponse](#minekube-gate-v1-UpdateLiteRouteStrategyResponse)
    - [ValidateConfigRequest](#minekube-gate-v1-ValidateConfigRequest)
    - [ValidateConfigResponse](#minekube-gate-v1-ValidateConfigResponse)
  
    - [ConfigFormat](#minekube-gate-v1-ConfigFormat)
    - [ProxyMode](#minekube-gate-v1-ProxyMode)
  
    - [GateService](#minekube-gate-v1-GateService)
  
- [Scalar Value Types](#scalar-value-types)



<a name="minekube_gate_v1_gate_service-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## minekube/gate/v1/gate_service.proto



<a name="minekube-gate-v1-APIConfig"></a>

### APIConfig
APIConfig represents the Gate API configuration


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enabled | [bool](#bool) |  |  |
| bind | [string](#string) |  |  |






<a name="minekube-gate-v1-AddLiteRouteBackendRequest"></a>

### AddLiteRouteBackendRequest
AddLiteRouteBackendRequest adds a backend to a route.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| host | [string](#string) |  |  |
| backend | [string](#string) |  |  |






<a name="minekube-gate-v1-AddLiteRouteBackendResponse"></a>

### AddLiteRouteBackendResponse
AddLiteRouteBackendResponse contains validation warnings.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| warnings | [string](#string) | repeated |  |






<a name="minekube-gate-v1-ApplyConfigRequest"></a>

### ApplyConfigRequest
ApplyConfigRequest is the request for ApplyConfig method.
Use either yaml_config for YAML configurations or json_config for structured JSON configurations.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| yaml_config | [string](#string) |  | YAML configuration as string |
| json_config | [GateConfig](#minekube-gate-v1-GateConfig) |  | JSON configuration as structured protobuf |
| persist | [bool](#bool) |  | Whether to persist the config to disk by overwriting the existing config file. Only works if a config file exists. Defaults to false (in-memory only). |






<a name="minekube-gate-v1-ApplyConfigResponse"></a>

### ApplyConfigResponse
ApplyConfigResponse contains validation warnings emitted while applying the config.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| warnings | [string](#string) | repeated |  |






<a name="minekube-gate-v1-BedrockPlayerData"></a>

### BedrockPlayerData
BedrockPlayerData contains information specific to Bedrock Edition players.
This data is only available for players connecting through Geyser/Floodgate.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| xuid | [int64](#int64) |  | Xbox User ID (XUID) - unique identifier for Bedrock players |
| device_os | [string](#string) |  | Device operating system the player is using |
| language | [string](#string) |  | Client language code (e.g., &#34;en_US&#34;) |
| ui_profile | [int32](#int32) |  | UI profile: 0 = Classic, 1 = Pocket |
| input_mode | [int32](#int32) |  | Input mode: 0 = Unknown, 1 = Mouse, 2 = Touch, 3 = GamePad, 4 = MotionController |
| behind_proxy | [bool](#bool) |  | Whether the player is connecting through a proxy |
| linked_player | [string](#string) |  | Linked Java Edition username (if any) |






<a name="minekube-gate-v1-ConnectConfig"></a>

### ConnectConfig
ConnectConfig represents the Connect network configuration


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enabled | [bool](#bool) |  |  |
| name | [string](#string) |  |  |
| allow_offline_mode_players | [bool](#bool) |  |  |






<a name="minekube-gate-v1-ConnectPlayerRequest"></a>

### ConnectPlayerRequest
ConnectPlayerRequest is the request for ConnectPlayer method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| player | [string](#string) |  | The player&#39;s username or ID to connect |
| server | [string](#string) |  | The target server name to connect the player to |






<a name="minekube-gate-v1-ConnectPlayerResponse"></a>

### ConnectPlayerResponse
ConnectPlayerResponse is the response for ConnectPlayer method.






<a name="minekube-gate-v1-DisconnectPlayerRequest"></a>

### DisconnectPlayerRequest
DisconnectPlayerRequest is the request for DisconnectPlayer method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| player | [string](#string) |  | The player&#39;s username or ID to disconnect |
| reason | [string](#string) |  | The reason displayed to the player when they are disconnected.

Formats:

- `{\&#34;text\&#34;:\&#34;Hello, world!\&#34;}` - JSON text component. See https://wiki.vg/Text_formatting for details.

- `§aHello,\n§bworld!` - Simple color codes. See https://wiki.vg/Text_formatting#Colors

Optional, if empty no reason will be shown. |






<a name="minekube-gate-v1-DisconnectPlayerResponse"></a>

### DisconnectPlayerResponse
DisconnectPlayerResponse is the response for DisconnectPlayer method.






<a name="minekube-gate-v1-ForwardingConfig"></a>

### ForwardingConfig
ForwardingConfig represents player info forwarding settings


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| mode | [string](#string) |  |  |
| velocity_secret | [string](#string) |  |  |
| bungee_guard_secret | [string](#string) |  |  |






<a name="minekube-gate-v1-GateConfig"></a>

### GateConfig
GateConfig represents the root configuration structure


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| api | [APIConfig](#minekube-gate-v1-APIConfig) |  |  |
| health_service | [HealthServiceConfig](#minekube-gate-v1-HealthServiceConfig) |  |  |
| connect | [ConnectConfig](#minekube-gate-v1-ConnectConfig) |  |  |
| config | [JavaConfig](#minekube-gate-v1-JavaConfig) |  |  |






<a name="minekube-gate-v1-GetConfigRequest"></a>

### GetConfigRequest
GetConfigRequest is the request for GetConfig method.
If format is unspecified JSON will be returned.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| format | [ConfigFormat](#minekube-gate-v1-ConfigFormat) |  |  |






<a name="minekube-gate-v1-GetConfigResponse"></a>

### GetConfigResponse
GetConfigResponse contains the serialized config payload.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| format | [ConfigFormat](#minekube-gate-v1-ConfigFormat) |  |  |
| payload | [string](#string) |  |  |






<a name="minekube-gate-v1-GetLiteRouteRequest"></a>

### GetLiteRouteRequest
GetLiteRouteRequest is the request for GetLiteRoute method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| host | [string](#string) |  | Host matcher to look up (case-insensitive). |






<a name="minekube-gate-v1-GetLiteRouteResponse"></a>

### GetLiteRouteResponse
GetLiteRouteResponse is the response for GetLiteRoute method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| route | [LiteRoute](#minekube-gate-v1-LiteRoute) |  |  |






<a name="minekube-gate-v1-GetPlayerRequest"></a>

### GetPlayerRequest
GetPlayerRequest is the request for GetPlayer method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Gets the player by their Minecraft UUID. Optional, if not set the username will be used. If both id and username are set, the id will be used. Must be a valid Minecraft UUID format (e.g. &#34;550e8400-e29b-41d4-a716-446655440000&#34;) |
| username | [string](#string) |  | Gets the player by their username. Optional, if not set the id will be used. Case-sensitive. |






<a name="minekube-gate-v1-GetPlayerResponse"></a>

### GetPlayerResponse
GetPlayerResponse is the response for GetPlayer method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| player | [Player](#minekube-gate-v1-Player) |  | The player matching the request criteria |






<a name="minekube-gate-v1-GetStatusRequest"></a>

### GetStatusRequest
GetStatusRequest is the request for GetStatus method.






<a name="minekube-gate-v1-GetStatusResponse"></a>

### GetStatusResponse
GetStatusResponse contains proxy runtime metadata.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| version | [string](#string) |  |  |
| mode | [ProxyMode](#minekube-gate-v1-ProxyMode) |  |  |
| players | [int32](#int32) |  |  |
| servers | [int32](#int32) |  |  |






<a name="minekube-gate-v1-HealthServiceConfig"></a>

### HealthServiceConfig
HealthServiceConfig represents the health service configuration


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enabled | [bool](#bool) |  |  |
| bind | [string](#string) |  |  |






<a name="minekube-gate-v1-JavaConfig"></a>

### JavaConfig
JavaConfig represents the main Java edition configuration


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| bind | [string](#string) |  |  |
| online_mode | [bool](#bool) |  |  |
| forwarding | [ForwardingConfig](#minekube-gate-v1-ForwardingConfig) |  |  |
| status | [StatusConfig](#minekube-gate-v1-StatusConfig) |  |  |
| servers | [JavaConfig.ServersEntry](#minekube-gate-v1-JavaConfig-ServersEntry) | repeated |  |
| try | [string](#string) | repeated |  |
| forced_hosts | [JavaConfig.ForcedHostsEntry](#minekube-gate-v1-JavaConfig-ForcedHostsEntry) | repeated |  |
| accept_transfers | [bool](#bool) |  |  |
| bungee_plugin_channel_enabled | [bool](#bool) |  |  |
| lite | [LiteConfig](#minekube-gate-v1-LiteConfig) |  | Add other commonly used fields as needed |






<a name="minekube-gate-v1-JavaConfig-ForcedHostsEntry"></a>

### JavaConfig.ForcedHostsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [StringList](#minekube-gate-v1-StringList) |  |  |






<a name="minekube-gate-v1-JavaConfig-ServersEntry"></a>

### JavaConfig.ServersEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="minekube-gate-v1-ListLiteRoutesRequest"></a>

### ListLiteRoutesRequest
ListLiteRoutesRequest is the request for ListLiteRoutes method.






<a name="minekube-gate-v1-ListLiteRoutesResponse"></a>

### ListLiteRoutesResponse
ListLiteRoutesResponse is the response for ListLiteRoutes method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| routes | [LiteRoute](#minekube-gate-v1-LiteRoute) | repeated |  |






<a name="minekube-gate-v1-ListPlayersRequest"></a>

### ListPlayersRequest
ListPlayersRequest is the request for ListPlayers method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| servers | [string](#string) | repeated | Filter players by server names. Optional, if empty all online players are returned. If specified, only returns players on the listed servers. |






<a name="minekube-gate-v1-ListPlayersResponse"></a>

### ListPlayersResponse
ListPlayersResponse is the response for ListPlayers method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| players | [Player](#minekube-gate-v1-Player) | repeated |  |






<a name="minekube-gate-v1-ListServersRequest"></a>

### ListServersRequest
ListServersRequest is the request for ListServers method.






<a name="minekube-gate-v1-ListServersResponse"></a>

### ListServersResponse
ListServersResponse is the response for ListServers method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| servers | [Server](#minekube-gate-v1-Server) | repeated |  |






<a name="minekube-gate-v1-LiteConfig"></a>

### LiteConfig
LiteConfig represents Gate Lite mode configuration


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enabled | [bool](#bool) |  |  |
| routes | [LiteRoute](#minekube-gate-v1-LiteRoute) | repeated |  |






<a name="minekube-gate-v1-LiteRoute"></a>

### LiteRoute
LiteRoute represents a configured lite route and runtime state.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hosts | [string](#string) | repeated |  |
| backends | [LiteRouteBackend](#minekube-gate-v1-LiteRouteBackend) | repeated |  |
| strategy | [string](#string) |  |  |
| options | [LiteRouteOptions](#minekube-gate-v1-LiteRouteOptions) |  |  |
| fallback | [LiteRouteFallback](#minekube-gate-v1-LiteRouteFallback) |  |  |






<a name="minekube-gate-v1-LiteRouteBackend"></a>

### LiteRouteBackend
LiteRouteBackend represents a backend target for a lite route.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| address | [string](#string) |  |  |
| active_connections | [uint32](#uint32) |  |  |






<a name="minekube-gate-v1-LiteRouteFallback"></a>

### LiteRouteFallback
LiteRouteFallback contains fallback response data served when all backends fail.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| motd_json | [string](#string) |  |  |
| version | [LiteRouteFallbackVersion](#minekube-gate-v1-LiteRouteFallbackVersion) |  |  |
| players | [LiteRouteFallbackPlayers](#minekube-gate-v1-LiteRouteFallbackPlayers) |  |  |
| favicon | [string](#string) |  |  |






<a name="minekube-gate-v1-LiteRouteFallbackPlayers"></a>

### LiteRouteFallbackPlayers
LiteRouteFallbackPlayers contains fallback player counts.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| online | [int32](#int32) |  |  |
| max | [int32](#int32) |  |  |






<a name="minekube-gate-v1-LiteRouteFallbackVersion"></a>

### LiteRouteFallbackVersion
LiteRouteFallbackVersion contains display version metadata.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| protocol | [int32](#int32) |  |  |






<a name="minekube-gate-v1-LiteRouteOptions"></a>

### LiteRouteOptions
LiteRouteOptions captures proxy behaviour flags for a lite route.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| proxy_protocol | [bool](#bool) |  |  |
| tcp_shield_real_ip | [bool](#bool) |  |  |
| modify_virtual_host | [bool](#bool) |  |  |
| cache_ping_ttl_ms | [int64](#int64) |  |  |






<a name="minekube-gate-v1-Player"></a>

### Player
Player represents an online player on the proxy.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | The player&#39;s Minecraft UUID |
| username | [string](#string) |  | The player&#39;s username |
| bedrock | [BedrockPlayerData](#minekube-gate-v1-BedrockPlayerData) |  | Optional Bedrock player data (only present for Bedrock players) |






<a name="minekube-gate-v1-RegisterServerRequest"></a>

### RegisterServerRequest
RegisterServerRequest is the request for RegisterServer method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | The unique name of the server |
| address | [string](#string) |  | The network address of the server (e.g. &#34;localhost:25565&#34;) |






<a name="minekube-gate-v1-RegisterServerResponse"></a>

### RegisterServerResponse
RegisterServerResponse is the response for RegisterServer method.






<a name="minekube-gate-v1-RemoveLiteRouteBackendRequest"></a>

### RemoveLiteRouteBackendRequest
RemoveLiteRouteBackendRequest removes a backend from a route.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| host | [string](#string) |  |  |
| backend | [string](#string) |  |  |






<a name="minekube-gate-v1-RemoveLiteRouteBackendResponse"></a>

### RemoveLiteRouteBackendResponse
RemoveLiteRouteBackendResponse contains validation warnings.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| warnings | [string](#string) | repeated |  |






<a name="minekube-gate-v1-RequestCookieRequest"></a>

### RequestCookieRequest
RequestCookieRequest is the request for RequestCookie method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| player | [string](#string) |  | The player&#39;s username or ID |
| key | [string](#string) |  | The key of the cookie in format `namespace:key` |






<a name="minekube-gate-v1-RequestCookieResponse"></a>

### RequestCookieResponse
RequestCookieResponse is the response for RequestCookie method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| payload | [bytes](#bytes) |  | The payload of the cookie. May be empty if the cookie is not found. |






<a name="minekube-gate-v1-Server"></a>

### Server
Server represents a backend server where Gate can connect players to.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | The unique name of the server. |
| address | [string](#string) |  | The network address of the server. |
| players | [int32](#int32) |  | The number of players currently on the server. |






<a name="minekube-gate-v1-StatusConfig"></a>

### StatusConfig
StatusConfig represents status response settings


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| motd | [string](#string) |  |  |
| show_max_players | [int32](#int32) |  |  |
| favicon | [string](#string) |  |  |
| log_ping_requests | [bool](#bool) |  |  |
| announce_forge | [bool](#bool) |  |  |






<a name="minekube-gate-v1-StoreCookieRequest"></a>

### StoreCookieRequest
StoreCookieRequest is the request for StoreCookie method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| player | [string](#string) |  | The player&#39;s username or ID |
| key | [string](#string) |  | The key of the cookie in format `namespace:key` |
| payload | [bytes](#bytes) |  | The payload to store. Passing an empty payload will remove the cookie. |






<a name="minekube-gate-v1-StoreCookieResponse"></a>

### StoreCookieResponse
StoreCookieResponse is the response for StoreCookie method.






<a name="minekube-gate-v1-StringList"></a>

### StringList
StringList helper for repeated string values in maps


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| values | [string](#string) | repeated |  |






<a name="minekube-gate-v1-UnregisterServerRequest"></a>

### UnregisterServerRequest
UnregisterServerRequest is the request for UnregisterServer method.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | The name of the server. Optional, if not set, the address will be used to match servers. |
| address | [string](#string) |  | The address of the server. Optional, if not set, the name will be used to match servers. If both name and address are set, only the server that matches both properties exactly will be unregistered. If only the address is set, the first server matching that address will be unregistered. |






<a name="minekube-gate-v1-UnregisterServerResponse"></a>

### UnregisterServerResponse
UnregisterServerResponse is the response for UnregisterServer method.






<a name="minekube-gate-v1-UpdateLiteRouteFallbackRequest"></a>

### UpdateLiteRouteFallbackRequest
UpdateLiteRouteFallbackRequest updates fallback metadata using a field mask.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| host | [string](#string) |  |  |
| fallback | [LiteRouteFallback](#minekube-gate-v1-LiteRouteFallback) |  |  |
| update_mask | [google.protobuf.FieldMask](#google-protobuf-FieldMask) |  |  |






<a name="minekube-gate-v1-UpdateLiteRouteFallbackResponse"></a>

### UpdateLiteRouteFallbackResponse
UpdateLiteRouteFallbackResponse contains validation warnings.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| warnings | [string](#string) | repeated |  |






<a name="minekube-gate-v1-UpdateLiteRouteOptionsRequest"></a>

### UpdateLiteRouteOptionsRequest
UpdateLiteRouteOptionsRequest updates per-route options using a field mask.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| host | [string](#string) |  |  |
| options | [LiteRouteOptions](#minekube-gate-v1-LiteRouteOptions) |  |  |
| update_mask | [google.protobuf.FieldMask](#google-protobuf-FieldMask) |  |  |






<a name="minekube-gate-v1-UpdateLiteRouteOptionsResponse"></a>

### UpdateLiteRouteOptionsResponse
UpdateLiteRouteOptionsResponse contains validation warnings.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| warnings | [string](#string) | repeated |  |






<a name="minekube-gate-v1-UpdateLiteRouteStrategyRequest"></a>

### UpdateLiteRouteStrategyRequest
UpdateLiteRouteStrategyRequest updates the load-balancing strategy for a route.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| host | [string](#string) |  |  |
| strategy | [string](#string) |  |  |






<a name="minekube-gate-v1-UpdateLiteRouteStrategyResponse"></a>

### UpdateLiteRouteStrategyResponse
UpdateLiteRouteStrategyResponse contains validation warnings.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| warnings | [string](#string) | repeated |  |






<a name="minekube-gate-v1-ValidateConfigRequest"></a>

### ValidateConfigRequest
ValidateConfigRequest is the request for ValidateConfig method.
Use either yaml_config for YAML configurations or json_config for structured JSON configurations.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| yaml_config | [string](#string) |  | YAML configuration as string |
| json_config | [GateConfig](#minekube-gate-v1-GateConfig) |  | JSON configuration as structured protobuf |






<a name="minekube-gate-v1-ValidateConfigResponse"></a>

### ValidateConfigResponse
ValidateConfigResponse contains validation warnings when the config is valid.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| warnings | [string](#string) | repeated |  |





 


<a name="minekube-gate-v1-ConfigFormat"></a>

### ConfigFormat
ConfigFormat specifies the serialization format for config payloads.

| Name | Number | Description |
| ---- | ------ | ----------- |
| CONFIG_FORMAT_UNSPECIFIED | 0 |  |
| CONFIG_FORMAT_JSON | 1 |  |
| CONFIG_FORMAT_YAML | 2 |  |



<a name="minekube-gate-v1-ProxyMode"></a>

### ProxyMode
ProxyMode enumerates the current operating mode of Gate.

| Name | Number | Description |
| ---- | ------ | ----------- |
| PROXY_MODE_UNSPECIFIED | 0 |  |
| PROXY_MODE_CLASSIC | 1 |  |
| PROXY_MODE_LITE | 2 |  |


 

 


<a name="minekube-gate-v1-GateService"></a>

### GateService
GateService is the service API for managing a Gate proxy instance.
It provides methods for managing players and servers.
All methods follow standard gRPC error codes and include detailed error messages.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| GetPlayer | [GetPlayerRequest](#minekube-gate-v1-GetPlayerRequest) | [GetPlayerResponse](#minekube-gate-v1-GetPlayerResponse) | GetPlayer returns the player by the given id or username. Returns NOT_FOUND if the player is not online. Returns INVALID_ARGUMENT if neither id nor username is provided, or if the id format is invalid. |
| ListPlayers | [ListPlayersRequest](#minekube-gate-v1-ListPlayersRequest) | [ListPlayersResponse](#minekube-gate-v1-ListPlayersResponse) | ListPlayers returns all online players. If servers are specified in the request, only returns players on those servers. |
| ListServers | [ListServersRequest](#minekube-gate-v1-ListServersRequest) | [ListServersResponse](#minekube-gate-v1-ListServersResponse) | ListServers returns all registered servers. |
| RegisterServer | [RegisterServerRequest](#minekube-gate-v1-RegisterServerRequest) | [RegisterServerResponse](#minekube-gate-v1-RegisterServerResponse) | RegisterServer adds a server to the proxy. Returns ALREADY_EXISTS if a server with the same name is already registered. Returns INVALID_ARGUMENT if the server name or address is invalid. |
| UnregisterServer | [UnregisterServerRequest](#minekube-gate-v1-UnregisterServerRequest) | [UnregisterServerResponse](#minekube-gate-v1-UnregisterServerResponse) | UnregisterServer removes a server from the proxy. Returns NOT_FOUND if no matching server is found. Returns INVALID_ARGUMENT if neither name nor address is provided. |
| ConnectPlayer | [ConnectPlayerRequest](#minekube-gate-v1-ConnectPlayerRequest) | [ConnectPlayerResponse](#minekube-gate-v1-ConnectPlayerResponse) | ConnectPlayer connects a player to a specified server. Returns NOT_FOUND if either the player or target server doesn&#39;t exist. Returns FAILED_PRECONDITION if the connection attempt fails. |
| DisconnectPlayer | [DisconnectPlayerRequest](#minekube-gate-v1-DisconnectPlayerRequest) | [DisconnectPlayerResponse](#minekube-gate-v1-DisconnectPlayerResponse) | DisconnectPlayer disconnects a player from the proxy. Returns NOT_FOUND if the player doesn&#39;t exist. Returns INVALID_ARGUMENT if the reason text is malformed. |
| StoreCookie | [StoreCookieRequest](#minekube-gate-v1-StoreCookieRequest) | [StoreCookieResponse](#minekube-gate-v1-StoreCookieResponse) | StoreCookie stores a cookie on a player&#39;s client. Returns NOT_FOUND if the player doesn&#39;t exist. Passing an empty payload will remove the cookie. |
| RequestCookie | [RequestCookieRequest](#minekube-gate-v1-RequestCookieRequest) | [RequestCookieResponse](#minekube-gate-v1-RequestCookieResponse) | RequestCookie requests a cookie from a player&#39;s client. The payload in RequestCookieResponse may be empty if the cookie is not found. |
| GetStatus | [GetStatusRequest](#minekube-gate-v1-GetStatusRequest) | [GetStatusResponse](#minekube-gate-v1-GetStatusResponse) | GetStatus returns current proxy metadata including version, mode, players and servers. |
| GetConfig | [GetConfigRequest](#minekube-gate-v1-GetConfigRequest) | [GetConfigResponse](#minekube-gate-v1-GetConfigResponse) | GetConfig returns the current effective config with secrets redacted. |
| ValidateConfig | [ValidateConfigRequest](#minekube-gate-v1-ValidateConfigRequest) | [ValidateConfigResponse](#minekube-gate-v1-ValidateConfigResponse) | ValidateConfig parses and validates a config payload without applying it. |
| ApplyConfig | [ApplyConfigRequest](#minekube-gate-v1-ApplyConfigRequest) | [ApplyConfigResponse](#minekube-gate-v1-ApplyConfigResponse) | ApplyConfig parses, validates, and applies a new config payload. |
| ListLiteRoutes | [ListLiteRoutesRequest](#minekube-gate-v1-ListLiteRoutesRequest) | [ListLiteRoutesResponse](#minekube-gate-v1-ListLiteRoutesResponse) | ListLiteRoutes returns lite routes and their active connection counters. |
| GetLiteRoute | [GetLiteRouteRequest](#minekube-gate-v1-GetLiteRouteRequest) | [GetLiteRouteResponse](#minekube-gate-v1-GetLiteRouteResponse) | GetLiteRoute returns detailed information about a single lite route. |
| UpdateLiteRouteStrategy | [UpdateLiteRouteStrategyRequest](#minekube-gate-v1-UpdateLiteRouteStrategyRequest) | [UpdateLiteRouteStrategyResponse](#minekube-gate-v1-UpdateLiteRouteStrategyResponse) | UpdateLiteRouteStrategy updates the load-balancing strategy for a lite route. |
| AddLiteRouteBackend | [AddLiteRouteBackendRequest](#minekube-gate-v1-AddLiteRouteBackendRequest) | [AddLiteRouteBackendResponse](#minekube-gate-v1-AddLiteRouteBackendResponse) | AddLiteRouteBackend adds a backend target to a lite route. |
| RemoveLiteRouteBackend | [RemoveLiteRouteBackendRequest](#minekube-gate-v1-RemoveLiteRouteBackendRequest) | [RemoveLiteRouteBackendResponse](#minekube-gate-v1-RemoveLiteRouteBackendResponse) | RemoveLiteRouteBackend removes a backend target from a lite route. |
| UpdateLiteRouteOptions | [UpdateLiteRouteOptionsRequest](#minekube-gate-v1-UpdateLiteRouteOptionsRequest) | [UpdateLiteRouteOptionsResponse](#minekube-gate-v1-UpdateLiteRouteOptionsResponse) | UpdateLiteRouteOptions updates proxy options for a lite route using a field mask. |
| UpdateLiteRouteFallback | [UpdateLiteRouteFallbackRequest](#minekube-gate-v1-UpdateLiteRouteFallbackRequest) | [UpdateLiteRouteFallbackResponse](#minekube-gate-v1-UpdateLiteRouteFallbackResponse) | UpdateLiteRouteFallback updates fallback metadata for a lite route using a field mask. |

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

