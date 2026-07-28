mod abi;
mod engine;
pub mod reentry;

use std::any::Any;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::ptr;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context as _, anyhow};

use abi::{AbiLimits, OwnedBytes, OwnedSample, SampleView, Slice, SliceList};
pub use engine::Engine;
pub use reentry::ActiveCall;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Sample {
    pub text: String,
    pub factor: i32,
    pub tags: Vec<String>,
}

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
    fn context_is_cancelled(&self, context: u64) -> anyhow::Result<bool>;

    fn proxy_transform(&self, proxy: u64, input: Sample) -> anyhow::Result<Result<Sample, String>>;

    fn proxy_emit_nested(
        &self,
        active: &mut ActiveCall<'_>,
        proxy: u64,
        input: String,
    ) -> anyhow::Result<Result<String, String>>;
}

struct CgoHost {
    handle: usize,
}

unsafe extern "C" {
    fn gate_wasm_go_context_cancelled(
        host: usize,
        context_id: u64,
        cancelled: *mut u8,
        error: *mut OwnedBytes,
    ) -> i32;
    fn gate_wasm_go_transform(
        host: usize,
        proxy_id: u64,
        input: SampleView,
        output: *mut OwnedSample,
        error: *mut OwnedBytes,
    ) -> i32;
    fn gate_wasm_go_emit_nested(
        host: usize,
        reentry: *mut GateWasmReentry,
        proxy_id: u64,
        input: Slice,
        output: *mut OwnedBytes,
        error: *mut OwnedBytes,
    ) -> i32;
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
    fn context_is_cancelled(&self, context: u64) -> anyhow::Result<bool> {
        let mut cancelled = 0_u8;
        let mut error = OwnedBytes::default();
        // SAFETY: All pointers reference stack values valid for the complete
        // synchronous callback. Go does not retain them.
        let status = unsafe {
            gate_wasm_go_context_cancelled(self.handle, context, &raw mut cancelled, &raw mut error)
        };
        if status != 0 {
            return Err(Self::callback_error("Go context callback", error));
        }
        Ok(cancelled != 0)
    }

    fn proxy_transform(&self, proxy: u64, input: Sample) -> anyhow::Result<Result<Sample, String>> {
        let tags: Vec<_> = input
            .tags
            .iter()
            .map(|tag| Slice {
                ptr: tag.as_ptr(),
                len: tag.len(),
            })
            .collect();
        let view = SampleView {
            text: Slice {
                ptr: input.text.as_ptr(),
                len: input.text.len(),
            },
            factor: input.factor,
            tags: SliceList {
                ptr: tags.as_ptr(),
                len: tags.len(),
            },
        };
        let mut output = OwnedSample::default();
        let mut error = OwnedBytes::default();
        // SAFETY: The view borrows input and tags only for this synchronous
        // callback. Go copies all values before returning.
        let status = unsafe {
            gate_wasm_go_transform(self.handle, proxy, view, &raw mut output, &raw mut error)
        };
        if status != 0 {
            return Err(Self::callback_error("Go transform callback", error));
        }
        // SAFETY: Successful Go callbacks transfer their C.malloc output.
        let output = unsafe { output.copy_and_free_c() }
            .context("failed to copy Go transform callback output")?;
        Ok(Ok(output))
    }

    fn proxy_emit_nested(
        &self,
        active: &mut ActiveCall<'_>,
        proxy: u64,
        input: String,
    ) -> anyhow::Result<Result<String, String>> {
        let mut output = OwnedBytes::default();
        let mut error = OwnedBytes::default();
        let input = Slice {
            ptr: input.as_ptr(),
            len: input.len(),
        };
        let reentry = ptr::from_mut(active).cast::<GateWasmReentry>();
        // SAFETY: reentry is valid only for this synchronous callback. The Go
        // adapter expires its wrapper before returning.
        let status = unsafe {
            gate_wasm_go_emit_nested(
                self.handle,
                reentry,
                proxy,
                input,
                &raw mut output,
                &raw mut error,
            )
        };
        if status != 0 {
            return Err(Self::callback_error("Go nested callback", error));
        }
        // SAFETY: Successful Go callbacks transfer their C.malloc output.
        let output = unsafe { output.copy_and_free_c() }
            .context("failed to copy Go nested callback output")?;
        Ok(Ok(
            String::from_utf8(output).context("Go nested callback output is not UTF-8")?
        ))
    }
}

#[repr(C)]
pub struct GateWasmRuntime {
    engine: Engine,
}

#[repr(C)]
pub struct GateWasmReentry {
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
/// `runtime` must be live and exclusively borrowed for this call; output
/// pointers must be writable.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_init(
    runtime: *mut GateWasmRuntime,
    context_id: u64,
    proxy_id: u64,
    output: *mut OwnedSample,
    error: *mut OwnedBytes,
) -> i32 {
    // SAFETY: ffi_status contains all panics and reports errors through C.
    unsafe {
        ffi_status(error, || {
            let runtime = runtime
                .as_mut()
                .ok_or_else(|| anyhow!("wasm runtime is null"))?;
            let result = runtime.engine.init(context_id, proxy_id)?;
            set_owned(output, OwnedSample::from_sample(result))
        })
    }
}

/// # Safety
///
/// `runtime` must be live and exclusively borrowed, `input` must be readable,
/// and output pointers must be writable for this call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_on_event(
    runtime: *mut GateWasmRuntime,
    proxy_id: u64,
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
            runtime.engine.ensure_transfer(input.len)?;
            let input = String::from_utf8(input.copy()?).context("event input is not UTF-8")?;
            let result = runtime.engine.on_event(proxy_id, &input)?;
            set_owned(output, OwnedBytes::from_string(result))
        })
    }
}

/// # Safety
///
/// `runtime` must be live and exclusively borrowed; output pointers must be
/// writable for this call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_allocate(
    runtime: *mut GateWasmRuntime,
    bytes: u64,
    output: *mut u64,
    error: *mut OwnedBytes,
) -> i32 {
    // SAFETY: ffi_status contains all panics and reports errors through C.
    unsafe {
        ffi_status(error, || {
            let runtime = runtime
                .as_mut()
                .ok_or_else(|| anyhow!("wasm runtime is null"))?;
            let result = runtime.engine.allocate(bytes)?;
            set_owned(output, result)
        })
    }
}

/// # Safety
///
/// `runtime` must be live and exclusively borrowed and `error` must be null or
/// writable for this call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_runtime_spin(
    runtime: *mut GateWasmRuntime,
    error: *mut OwnedBytes,
) -> i32 {
    // SAFETY: ffi_status contains all panics and reports errors through C.
    unsafe {
        ffi_status(error, || {
            let runtime = runtime
                .as_mut()
                .ok_or_else(|| anyhow!("wasm runtime is null"))?;
            runtime.engine.spin()
        })
    }
}

/// # Safety
///
/// `reentry` must be the active token supplied to the current host callback;
/// input and output pointers must be valid for this call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_reentry_on_event(
    reentry: *mut GateWasmReentry,
    proxy_id: u64,
    input: Slice,
    output: *mut OwnedBytes,
    error: *mut OwnedBytes,
) -> i32 {
    // SAFETY: ffi_status contains all panics and reports errors through C.
    unsafe {
        ffi_status(error, || {
            let reentry = reentry
                .cast::<ActiveCall<'static>>()
                .as_mut()
                .ok_or_else(|| anyhow!("wasm reentry token is null"))?;
            reentry.ensure_transfer(input.len)?;
            let input = String::from_utf8(input.copy()?).context("event input is not UTF-8")?;
            let result = reentry.on_event(proxy_id, &input)?;
            set_owned(output, OwnedBytes::from_string(result))
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

/// # Safety
///
/// `value` must be an output returned by this library and not already freed.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn gate_wasm_owned_sample_free(value: OwnedSample) {
    let _ = catch_unwind(AssertUnwindSafe(|| {
        // SAFETY: The caller returns an owned Rust output exactly once.
        unsafe { value.free_rust() };
    }));
}
