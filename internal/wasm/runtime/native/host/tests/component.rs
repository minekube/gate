use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use gate_wasm_native::wire::{
    Response, WireValue, decode_request, decode_response, encode_request, encode_response,
};
use gate_wasm_native::{CallbackCall, Engine, Host, Limits};

const COMPONENT: &[u8] = include_bytes!("../../artifacts/gate_wasm_fixture.component.wasm");
const CALLBACK_ERROR_FUNC: u32 = 8;
const CALLBACK_HANDLE: u64 = 3;
const CALLBACK_GUEST_ID: u64 = 42;

fn operation_id(identity: &str) -> u32 {
    gate_wasm_native::generated::dispatch::OPERATIONS
        .iter()
        .find(|operation| operation.identity == identity)
        .unwrap_or_else(|| panic!("generated operation {identity} is missing"))
        .id
}

struct TestHost {
    invokes: Arc<AtomicUsize>,
}

impl Host for TestHost {
    fn invoke(
        &self,
        active: &mut CallbackCall<'_>,
        operation: u32,
        request: &[u8],
    ) -> anyhow::Result<Vec<u8>> {
        self.invokes.fetch_add(1, Ordering::SeqCst);
        if operation
            == operation_id("go.minekube.com/gate/pkg/edition/java/proxy.Proxy.PlayerCount")
        {
            assert_eq!(decode_request(request)?, [WireValue::Resource(2)]);
            let mut response = vec![1, 0, 1, 9];
            response.extend_from_slice(&0_i64.to_le_bytes());
            Ok(response)
        } else if operation
            == operation_id("go.minekube.com/gate/pkg/edition/java/proto/util.RecoverFunc")
        {
            assert_eq!(
                decode_request(request)?,
                [WireValue::Resource(CALLBACK_HANDLE)]
            );
            let callback = active.invoke_callback(
                CALLBACK_ERROR_FUNC,
                CALLBACK_GUEST_ID,
                &encode_request(&[])?,
            )?;
            assert_eq!(decode_response(&callback)?.error, None);
            encode_response(&Response {
                values: vec![],
                error: None,
            })
        } else {
            anyhow::bail!("unexpected generated operation {operation}")
        }
    }

    fn register_callback(&self, callback_type: u32, guest_id: u64) -> anyhow::Result<u64> {
        assert_eq!(callback_type, CALLBACK_ERROR_FUNC);
        assert_eq!(guest_id, CALLBACK_GUEST_ID);
        Ok(CALLBACK_HANDLE)
    }
}

#[test]
fn init_receives_real_gate_resources_and_calls_generated_api() -> anyhow::Result<()> {
    let invokes = Arc::new(AtomicUsize::new(0));
    let host = Arc::new(TestHost {
        invokes: Arc::clone(&invokes),
    });
    let mut engine = Engine::new(COMPONENT, host, Limits::default())?;

    engine.init(1, 2)?;

    assert_eq!(invokes.load(Ordering::SeqCst), 2);
    Ok(())
}

struct DropHost {
    drops: Arc<AtomicUsize>,
}

impl Drop for DropHost {
    fn drop(&mut self) {
        self.drops.fetch_add(1, Ordering::SeqCst);
    }
}

impl Host for DropHost {
    fn invoke(
        &self,
        active: &mut CallbackCall<'_>,
        operation: u32,
        request: &[u8],
    ) -> anyhow::Result<Vec<u8>> {
        if operation
            == operation_id("go.minekube.com/gate/pkg/edition/java/proxy.Proxy.PlayerCount")
        {
            let mut response = vec![1, 0, 1, 9];
            response.extend_from_slice(&0_i64.to_le_bytes());
            Ok(response)
        } else if operation
            == operation_id("go.minekube.com/gate/pkg/edition/java/proto/util.RecoverFunc")
        {
            let callback = active.invoke_callback(
                CALLBACK_ERROR_FUNC,
                CALLBACK_GUEST_ID,
                &encode_request(&[])?,
            )?;
            assert_eq!(decode_response(&callback)?.error, None);
            encode_response(&Response {
                values: vec![],
                error: None,
            })
        } else {
            anyhow::bail!("unexpected generated operation {operation} with request {request:?}")
        }
    }

    fn register_callback(&self, _callback_type: u32, _guest_id: u64) -> anyhow::Result<u64> {
        Ok(CALLBACK_HANDLE)
    }
}

#[test]
fn dropping_engine_releases_host_exactly_once() -> anyhow::Result<()> {
    let drops = Arc::new(AtomicUsize::new(0));
    let host = Arc::new(DropHost {
        drops: Arc::clone(&drops),
    });
    let mut engine = Engine::new(COMPONENT, host.clone(), Limits::default())?;
    engine.init(1, 2)?;
    drop(host);

    assert_eq!(drops.load(Ordering::SeqCst), 0);
    drop(engine);
    assert_eq!(drops.load(Ordering::SeqCst), 1);
    Ok(())
}
