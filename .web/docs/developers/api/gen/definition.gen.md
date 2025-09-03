---
title: 'Gate Protocol Documentation - Generated API Reference'
description: 'Generated protocol documentation for Gate Minecraft proxy API. Complete reference for all API endpoints, messages, and data structures.'
---

# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [minekube/gate/v1/gate_service.proto](#minekube_gate_v1_gate_service-proto)
    - [ConnectPlayerRequest](#minekube-gate-v1-ConnectPlayerRequest)
    - [ConnectPlayerResponse](#minekube-gate-v1-ConnectPlayerResponse)
    - [DisconnectPlayerRequest](#minekube-gate-v1-DisconnectPlayerRequest)
    - [DisconnectPlayerResponse](#minekube-gate-v1-DisconnectPlayerResponse)
    - [GetPlayerRequest](#minekube-gate-v1-GetPlayerRequest)
    - [GetPlayerResponse](#minekube-gate-v1-GetPlayerResponse)
    - [ListPlayersRequest](#minekube-gate-v1-ListPlayersRequest)
    - [ListPlayersResponse](#minekube-gate-v1-ListPlayersResponse)
    - [ListServersRequest](#minekube-gate-v1-ListServersRequest)
    - [ListServersResponse](#minekube-gate-v1-ListServersResponse)
    - [Player](#minekube-gate-v1-Player)
    - [RegisterServerRequest](#minekube-gate-v1-RegisterServerRequest)
    - [RegisterServerResponse](#minekube-gate-v1-RegisterServerResponse)
    - [RequestCookieRequest](#minekube-gate-v1-RequestCookieRequest)
    - [RequestCookieResponse](#minekube-gate-v1-RequestCookieResponse)
    - [Server](#minekube-gate-v1-Server)
    - [StoreCookieRequest](#minekube-gate-v1-StoreCookieRequest)
    - [StoreCookieResponse](#minekube-gate-v1-StoreCookieResponse)
    - [UnregisterServerRequest](#minekube-gate-v1-UnregisterServerRequest)
    - [UnregisterServerResponse](#minekube-gate-v1-UnregisterServerResponse)
  
    - [GateService](#minekube-gate-v1-GateService)
  
- [Scalar Value Types](#scalar-value-types)



<a name="minekube_gate_v1_gate_service-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## minekube/gate/v1/gate_service.proto



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

- `{&#34;text&#34;:&#34;Hello, world!&#34;}` - JSON text component. See https://wiki.vg/Text_formatting for details.

- `§aHello,\n§bworld!` - Simple color codes. See https://wiki.vg/Text_formatting#Colors

Optional, if empty no reason will be shown. |






<a name="minekube-gate-v1-DisconnectPlayerResponse"></a>

### DisconnectPlayerResponse
DisconnectPlayerResponse is the response for DisconnectPlayer method.






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






<a name="minekube-gate-v1-Player"></a>

### Player
Player represents an online player on the proxy.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | The player&#39;s Minecraft UUID |
| username | [string](#string) |  | The player&#39;s username |






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

