wit_bindgen::generate!({
    path: "../../../api",
    world: "gate-plugin",
});

struct Fixture;

use exports::minekube::gate::plugin::*;

macro_rules! ignore_event_handlers {
    ($($name:ident => $event:ty),+ $(,)?) => {
        $(
            fn $name(_id: u64, _event: &$event) -> Result<(), GateError> {
                Ok(())
            }
        )+
    };
}

impl Guest for Fixture {
    fn metadata() -> PluginMetadata {
        PluginMetadata {
            name: "gate-wasm-fixture".into(),
            version: "0.0.0".into(),
            contract_hash: "05844571b07f3e19787939aa5fa98328c13acef7ab49ba74bb3388ab713cd2c3"
                .into(),
            generator_format: 1,
        }
    }

    fn init(_context: &ContextE30d9213847b, proxy: &Proxy3cf24d6ad4bb) -> Result<(), GateError> {
        let count = minekube::gate::pkg_edition_java_proxy::proxy_player_count(proxy);
        if count != 0 {
            return Err(GateError {
                kind: "fixture".into(),
                message: format!("expected empty proxy, got {count} players"),
                operation: "init".into(),
            });
        }
        let callback = minekube::gate::gate_callbacks::new_callback_9b79f3eb4945(42);
        minekube::gate::pkg_edition_java_proto_util::recover_func(Some(callback))?;
        Ok(())
    }

    fn invoke_callback_0105467f2bef(_id: u64) {}
    fn invoke_callback_1178afb46377(_id: u64, _arg0: Option<&GateType>) -> bool {
        false
    }
    fn invoke_callback_1f86eef915ce(_id: u64, _arg0: Option<&ContextE30d9213847b>) -> bool {
        false
    }
    fn invoke_callback_2053c44117ea(_id: u64, _arg0: Method) -> bool {
        false
    }
    fn invoke_callback_2c1c5989286b(_id: u64, _arg0: StructField) -> bool {
        false
    }
    fn invoke_callback_4e5db8a68395(
        _id: u64,
        _wr: Option<&Writer38b5996d575b>,
        _v: Option<&AnonymousDynamic3f3a2bbef3d45970>,
        _protocol: ProtocolE8c68391e5ba,
    ) -> Result<(), GateError> {
        Ok(())
    }
    fn invoke_callback_8ce8588e75ea(_id: u64, _c: Option<&RequiresContextPointer>) -> bool {
        false
    }
    fn invoke_callback_9b79f3eb4945(id: u64) -> Result<(), GateError> {
        if id == 42 {
            Ok(())
        } else {
            Err(GateError {
                kind: "fixture".into(),
                message: format!("unexpected callback ID {id}"),
                operation: "invoke-callback-9b79f3eb4945".into(),
            })
        }
    }
    fn invoke_callback_a3de14505458(
        _id: u64,
        _c: Option<&ContextPointer>,
    ) -> Result<(), GateError> {
        Ok(())
    }
    fn invoke_callback_a5d0afa3c957(_id: u64, _p: Option<&Player9f25ee10b6eb>) -> bool {
        false
    }
    fn invoke_callback_bf78377c58d5(_id: u64, _arg0: String) -> bool {
        false
    }
    fn invoke_callback_d4c80bbf993f(
        _id: u64,
        _rd: Option<&Reader4d25c51b00a6>,
        _protocol: ProtocolE8c68391e5ba,
    ) -> Result<Option<AnonymousDynamic3f3a2bbef3d45970>, GateError> {
        Ok(None)
    }
    fn invoke_callback_f39d53d38ffd(_id: u64, _arg0: Option<&Packet>) -> Result<(), GateError> {
        Ok(())
    }
    fn invoke_cancel_func(_id: u64) {}
    fn invoke_command_func(_id: u64, _c: Option<&CommandContext>) -> Result<(), GateError> {
        Ok(())
    }
    fn invoke_gate_func(_id: u64, _permission: String) -> TriState {
        0
    }
    fn invoke_has_joined_url_fn(
        _id: u64,
        _server_id: String,
        _username: String,
        _user_ip: String,
    ) -> String {
        String::new()
    }
    fn invoke_load_config_func(_id: u64) -> Result<Option<ConfigPointer37c11021fd63>, GateError> {
        Ok(None)
    }
    fn invoke_runnable_func(_id: u64, _ctx: Option<&ContextE30d9213847b>) -> Result<(), GateError> {
        Ok(())
    }
    fn invoke_seq_4454e9c05542(_id: u64, _yield: Option<Callback2053c44117ea>) {}
    fn invoke_seq_56a88aa8d577(_id: u64, _yield: Option<Callback1178afb46377>) {}
    fn invoke_seq_b074101856aa(_id: u64, _yield: Option<Callback2c1c5989286b>) {}
    fn invoke_start_option(_id: u64, _o: Option<&StartOptions>) {}
    fn invoke_suggest_func(
        _id: u64,
        _c: Option<&ContextPointer>,
        _b: Option<&SuggestionsBuilderPointer>,
    ) -> Option<SuggestionsPointer> {
        None
    }

    ignore_event_handlers! {
        invoke_command_execute_event_handler => CommandExecuteEvent,
        invoke_connection_event_handler => ConnectionEvent,
        invoke_connection_handshake_event_handler => ConnectionHandshakeEvent,
        invoke_cookie_receive_event_handler => CookieReceiveEvent,
        invoke_cookie_request_event_handler => CookieRequestEvent,
        invoke_cookie_store_event_handler => CookieStoreEvent,
        invoke_disconnect_event_handler => DisconnectEvent,
        invoke_game_profile_request_event_handler => GameProfileRequestEvent,
        invoke_kicked_from_server_event_handler => KickedFromServerEvent,
        invoke_login_event_handler => LoginEvent,
        invoke_permissions_setup_event_handler => PermissionsSetupEvent,
        invoke_ping_event_handler => PingEvent,
        invoke_player_available_commands_event_handler => PlayerAvailableCommandsEvent,
        invoke_player_channel_register_event_handler => PlayerChannelRegisterEvent,
        invoke_player_channel_unregister_event_handler => PlayerChannelUnregisterEvent,
        invoke_player_chat_event_handler => PlayerChatEvent,
        invoke_player_choose_initial_server_event_handler => PlayerChooseInitialServerEvent,
        invoke_player_client_brand_event_handler => PlayerClientBrandEvent,
        invoke_player_mod_info_event_handler => PlayerModInfoEvent,
        invoke_player_resource_pack_status_event_handler => PlayerResourcePackStatusEvent6e0ee0072fce,
        invoke_player_settings_changed_event_handler => PlayerSettingsChangedEvent,
        invoke_plugin_message_event_handler => PluginMessageEvent,
        invoke_post_login_event_handler => PostLoginEvent,
        invoke_pre_login_event_handler => PreLoginEvent,
        invoke_pre_shutdown_event_handler => PreShutdownEvent,
        invoke_pre_transfer_event_handler => PreTransferEvent,
        invoke_ready_event_handler => ReadyEvent,
        invoke_server_connected_event_handler => ServerConnectedEvent,
        invoke_server_login_plugin_message_event_handler => ServerLoginPluginMessageEvent,
        invoke_server_post_connect_event_handler => ServerPostConnectEvent,
        invoke_server_pre_connect_event_handler => ServerPreConnectEvent,
        invoke_server_registered_event_handler => ServerRegisteredEvent,
        invoke_server_resource_pack_remove_event_handler => ServerResourcePackRemoveEvent,
        invoke_server_resource_pack_send_event_handler => ServerResourcePackSendEvent,
        invoke_server_unregistered_event_handler => ServerUnregisteredEvent,
        invoke_shutdown_event_handler => ShutdownEventPointer,
        invoke_tab_complete_event_handler => TabCompleteEvent,
    }
}

export!(Fixture);
