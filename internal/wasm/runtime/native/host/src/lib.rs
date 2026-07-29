pub mod abi;
mod engine;
pub mod generated;
#[doc(hidden)]
pub mod wire;

use std::any::Any;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::ptr;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context as _, anyhow};

use abi::{AbiLimits, OwnedBytes, Slice};
pub use engine::{CallbackCall, Engine, PluginMetadata};

#[derive(Clone, Copy, Debug)]
pub struct Limits {
    pub memory_bytes: usize,
    pub transfer_bytes: usize,
    pub fuel: u64,
    pub deadline: Duration,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            memory_bytes: 128 * 1024 * 1024,
            transfer_bytes: 16 * 1024 * 1024,
            fuel: 10_000_000,
            deadline: Duration::from_millis(100),
        }
    }
}

#[derive(Debug)]
pub(crate) struct MemoryLimitError;

impl std::fmt::Display for MemoryLimitError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("component exceeded its memory limit")
    }
}

impl std::error::Error for MemoryLimitError {}

#[derive(Debug)]
pub(crate) struct TransferLimitError;

impl std::fmt::Display for TransferLimitError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("component call exceeded its transfer limit")
    }
}

impl std::error::Error for TransferLimitError {}

pub trait Host: Send + Sync + 'static {
    fn invoke(
        &self,
        active: &mut CallbackCall<'_>,
        operation: u32,
        input: &[u8],
    ) -> anyhow::Result<Vec<u8>> {
        let _ = (active, operation, input);
        Err(anyhow!("generated Gate dispatch is unavailable"))
    }

    fn register_callback(&self, callback_type: u32, guest_id: u64) -> anyhow::Result<u64> {
        let _ = (callback_type, guest_id);
        Err(anyhow!("generated Gate callback dispatch is unavailable"))
    }

    fn drop_resource(&self, _handle: u64) -> anyhow::Result<()> {
        Ok(())
    }
}

struct CgoHost {
    handle: usize,
}

unsafe extern "C" {
    fn gate_wasm_go_invoke(
        host: usize,
        reentry: *mut GateWasmCallbackReentry,
        operation_id: u32,
        input: Slice,
        output: *mut OwnedBytes,
        error: *mut OwnedBytes,
    ) -> i32;
    fn gate_wasm_go_register_callback(
        host: usize,
        callback_type_id: u32,
        guest_id: u64,
        handle: *mut u64,
        error: *mut OwnedBytes,
    ) -> i32;
    fn gate_wasm_go_drop_resource(host: usize, handle: u64, error: *mut OwnedBytes) -> i32;
}

impl CgoHost {
    fn callback_error(operation: &str, error: OwnedBytes) -> anyhow::Error {
        // SAFETY: Go callback errors are allocated with C.malloc and transfer
        // ownership to Rust before the callback returns.
        match unsafe { error.copy_and_free_c() } {
            Ok(bytes) if !bytes.is_empty() => {
                anyhow!("{operation}: {}", String::from_utf8_lossy(&bytes))
            }
            Ok(_) => anyhow!("{operation} failed without an error message"),
            Err(copy_error) => anyhow!("{operation}: {copy_error}"),
        }
    }
}

impl Host for CgoHost {
    fn invoke(
        &self,
        active: &mut CallbackCall<'_>,
        operation: u32,
        input: &[u8],
    ) -> anyhow::Result<Vec<u8>> {
        let mut output = OwnedBytes::default();
        let mut error = OwnedBytes::default();
        let input = Slice {
            ptr: input.as_ptr(),
            len: input.len(),
        };
        // SAFETY: The input is borrowed only for this synchronous call and Go
        // transfers any output allocation to Rust.
        let reentry = ptr::from_mut(active).cast::<GateWasmCallbackReentry>();
        let status = unsafe {
            gate_wasm_go_invoke(
                self.handle,
                reentry,
                operation,
                input,
                &raw mut output,
                &raw mut error,
            )
        };
        if status != 0 {
            return Err(Self::callback_error("Go Gate API callback", error));
        }
        // SAFETY: Successful Go callbacks transfer their C.malloc output.
        unsafe { output.copy_and_free_c() }.context("failed to copy Go Gate API callback output")
    }

    fn drop_resource(&self, handle: u64) -> anyhow::Result<()> {
        let mut error = OwnedBytes::default();
        // SAFETY: The error output is valid for the complete synchronous call.
        let status = unsafe { gate_wasm_go_drop_resource(self.handle, handle, &raw mut error) };
        if status != 0 {
            return Err(Self::callback_error("Go resource drop callback", error));
        }
        Ok(())
    }

    fn register_callback(&self, callback_type: u32, guest_id: u64) -> anyhow::Result<u64> {
        let mut handle = 0_u64;
        let mut error = OwnedBytes::default();
        // SAFETY: The outputs are valid for the complete synchronous call.
        let status = unsafe {
            gate_wasm_go_register_callback(
                self.handle,
                callback_type,
                guest_id,
                &raw mut handle,
                &raw mut error,
            )
        };
        if status != 0 {
            return Err(Self::callback_error("Go callback registration", error));
        }
        Ok(handle)
    }
}

#[repr(C)]
pub struct GateWasmRuntime {
    engine: Engine,
}

#[repr(C)]
pub struct PluginMetadataView {
    name: Slice,
    version: Slice,
    contract_hash: Slice,
    generator_format: u32,
}

impl PluginMetadataView {
    fn new(metadata: &PluginMetadata) -> Self {
        Self {
            name: Slice {
                ptr: metadata.name.as_ptr(),
                len: metadata.name.len(),
            },
            version: Slice {
                ptr: metadata.version.as_ptr(),
                len: metadata.version.len(),
            },
            contract_hash: Slice {
                ptr: metadata.contract_hash.as_ptr(),
                len: metadata.contract_hash.len(),
            },
            generator_format: metadata.generator_format,
        }
    }
}

#[repr(C)]
pub struct GateWasmCallbackReentry {
    _opaque: [u8; 0],
}

fn limits_from_abi(limits: AbiLimits) -> anyhow::Result<Limits> {
    let defaults = Limits::default();
    Ok(Limits {
        memory_bytes: if limits.memory_bytes == 0 {
            defaults.memory_bytes
        } else {
            usize::try_from(limits.memory_bytes).context("memory limit does not fit usize")?
        },
        transfer_bytes: if limits.transfer_bytes == 0 {
            defaults.transfer_bytes
        } else {
            usize::try_from(limits.transfer_bytes).context("transfer limit does not fit usize")?
        },
        fuel: if limits.fuel == 0 {
            defaults.fuel
        } else {
            limits.fuel
        },
        deadline: if limits.deadline_nanos == 0 {
            defaults.deadline
        } else {
            Duration::from_nanos(limits.deadline_nanos)
        },
    })
}

fn panic_message(payload: Box<dyn Any + Send>) -> String {
    if let Some(message) = payload.downcast_ref::<&str>() {
        (*message).to_owned()
    } else if let Some(message) = payload.downcast_ref::<String>() {
        message.clone()
    } else {
        "Rust panic crossed the Wasm runtime boundary".to_owned()
    }
}

fn error_status(failure: &anyhow::Error) -> i32 {
    if let Some(trap) = failure.downcast_ref::<wasmtime::Trap>() {
        match trap {
            wasmtime::Trap::OutOfFuel => return 3,
            wasmtime::Trap::Interrupt => return 4,
            _ => {}
        }
    }
    if failure.downcast_ref::<MemoryLimitError>().is_some() {
        return 5;
    }
    if failure.downcast_ref::<TransferLimitError>().is_some() {
        return 6;
    }
    1
}

unsafe fn set_owned<T>(output: *mut T, value: T) -> anyhow::Result<()> {
    if output.is_null() {
        return Err(anyhow!("required output pointer is null"));
    }
    // SAFETY: The caller supplied a writable output location under the C ABI.
    unsafe { output.write(value) };
    Ok(())
}

unsafe fn set_error(error: *mut OwnedBytes, message: String) {
    if !error.is_null() {
        // SAFETY: The caller supplied a writable error output location.
        unsafe { error.write(OwnedBytes::from_string(message)) };
    }
}

unsafe fn ffi_status(
    error: *mut OwnedBytes,
    operation: impl FnOnce() -> anyhow::Result<()>,
) -> i32 {
    if !error.is_null() {
        // SAFETY: The caller supplied a writable error output location.
        unsafe { error.write(OwnedBytes::default()) };
    }
    match catch_unwind(AssertUnwindSafe(operation)) {
        Ok(Ok(())) => 0,
        Ok(Err(failure)) => {
            let status = error_status(&failure);
            // SAFETY: The caller owns the returned Rust buffer.
            unsafe { set_error(error, format!("{failure:#}")) };
            status
        }
        Err(payload) => {
            // SAFETY: The caller owns the returned Rust buffer.
            unsafe { set_error(error, panic_message(payload)) };
            2
        }
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn gate_wasm_runtime_version() -> Slice {
    const VERSION: &[u8] = b"wasmtime-47.0.2";
    Slice {
        ptr: VERSION.as_ptr(),
        len: VERSION.len(),
    }
}

/// Creates a runtime. A null return indicates failure and `error` owns the
/// diagnostic buffer.
///
/// # Safety
///
/// All non-null pointers must satisfy the ownership and lifetime contract in
/// `gate_wasm_native.h`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_new(
    component: Slice,
    go_host: usize,
    limits: AbiLimits,
    error: *mut OwnedBytes,
) -> *mut GateWasmRuntime {
    if !error.is_null() {
        // SAFETY: The caller supplied a writable error output location.
        unsafe { error.write(OwnedBytes::default()) };
    }
    let result = catch_unwind(AssertUnwindSafe(|| -> anyhow::Result<_> {
        if go_host == 0 {
            return Err(anyhow!("Go host handle is zero"));
        }
        // SAFETY: The C ABI guarantees the component slice for this call.
        let component = unsafe { component.copy() }?;
        let limits = limits_from_abi(limits)?;
        let host = Arc::new(CgoHost { handle: go_host });
        let engine = Engine::new(&component, host, limits)?;
        Ok(Box::into_raw(Box::new(GateWasmRuntime { engine })))
    }));
    match result {
        Ok(Ok(runtime)) => runtime,
        Ok(Err(failure)) => {
            // SAFETY: The caller owns the returned Rust buffer.
            unsafe { set_error(error, format!("{failure:#}")) };
            ptr::null_mut()
        }
        Err(payload) => {
            // SAFETY: The caller owns the returned Rust buffer.
            unsafe { set_error(error, panic_message(payload)) };
            ptr::null_mut()
        }
    }
}

/// # Safety
///
/// `runtime` must be live for this call and `output` must be writable. The
/// returned slices borrow runtime-owned strings and are valid until the runtime
/// is freed.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_metadata(
    runtime: *const GateWasmRuntime,
    output: *mut PluginMetadataView,
    error: *mut OwnedBytes,
) -> i32 {
    // SAFETY: ffi_status contains all panics and reports errors through C.
    unsafe {
        ffi_status(error, || {
            let runtime = runtime
                .as_ref()
                .ok_or_else(|| anyhow!("wasm runtime is null"))?;
            set_owned(output, PluginMetadataView::new(runtime.engine.metadata()))
        })
    }
}

/// # Safety
///
/// `runtime` must be live and exclusively borrowed for this call; output
/// pointers must be writable.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_init(
    runtime: *mut GateWasmRuntime,
    context_id: u64,
    proxy_id: u64,
    error: *mut OwnedBytes,
) -> i32 {
    // SAFETY: ffi_status contains all panics and reports errors through C.
    unsafe {
        ffi_status(error, || {
            let runtime = runtime
                .as_mut()
                .ok_or_else(|| anyhow!("wasm runtime is null"))?;
            runtime.engine.init(context_id, proxy_id)
        })
    }
}

/// # Safety
///
/// `runtime` must be live and exclusively borrowed, `input` must be readable,
/// and output pointers must be writable for this call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_invoke_callback(
    runtime: *mut GateWasmRuntime,
    callback_type_id: u32,
    guest_id: u64,
    input: Slice,
    output: *mut OwnedBytes,
    error: *mut OwnedBytes,
) -> i32 {
    // SAFETY: ffi_status contains all panics and reports errors through C.
    unsafe {
        ffi_status(error, || {
            let runtime = runtime
                .as_mut()
                .ok_or_else(|| anyhow!("wasm runtime is null"))?;
            let input = input.copy()?;
            let result = runtime
                .engine
                .invoke_callback(callback_type_id, guest_id, &input)?;
            set_owned(output, OwnedBytes::from_vec(result))
        })
    }
}

/// # Safety
///
/// `reentry` must be the active token supplied to the current generated host
/// call; input and output pointers must be valid for this call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_callback_reentry_invoke(
    reentry: *mut GateWasmCallbackReentry,
    callback_type_id: u32,
    guest_id: u64,
    input: Slice,
    output: *mut OwnedBytes,
    error: *mut OwnedBytes,
) -> i32 {
    // SAFETY: ffi_status contains all panics and reports errors through C.
    unsafe {
        ffi_status(error, || {
            let reentry = reentry
                .cast::<CallbackCall<'static>>()
                .as_mut()
                .ok_or_else(|| anyhow!("wasm callback reentry token is null"))?;
            let input = input.copy()?;
            let result = reentry.invoke_callback(callback_type_id, guest_id, &input)?;
            set_owned(output, OwnedBytes::from_vec(result))
        })
    }
}

/// # Safety
///
/// `runtime` must be null or an allocation returned by
/// `gate_wasm_runtime_new` that has not already been freed.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_free(runtime: *mut GateWasmRuntime) {
    if runtime.is_null() {
        return;
    }
    let _ = catch_unwind(AssertUnwindSafe(|| {
        // SAFETY: Ownership of this Box is returned exactly once by the C ABI.
        drop(unsafe { Box::from_raw(runtime) });
    }));
}

/// # Safety
///
/// `value` must be an output returned by this library and not already freed.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_owned_bytes_free(value: OwnedBytes) {
    let _ = catch_unwind(AssertUnwindSafe(|| {
        // SAFETY: The caller returns an owned Rust output exactly once.
        unsafe { value.free_rust() };
    }));
}
