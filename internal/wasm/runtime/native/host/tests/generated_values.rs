use gate_wasm_native::generated::dispatch::{
    CallbackDescriptor, Dispatch, Operation, ResourceDescriptor, WIT_HASH, add_to_linker,
};
use gate_wasm_native::generated::values::{
    ABI_LAYOUT_FINGERPRINT, ABI_SCHEMA_VERSION, VALUE_LAYOUTS,
};
use gate_wasm_native::{Sample, abi::OwnedSample};
use wasmtime::StoreContextMut;
use wasmtime::component::{Linker, Val, types::ComponentFunc};

#[test]
fn generated_values_have_valid_fixed_width_layouts() {
    assert_eq!(ABI_SCHEMA_VERSION, 1);
    assert_eq!(ABI_LAYOUT_FINGERPRINT.len(), 64);
    assert!(!VALUE_LAYOUTS.is_empty());

    for layout in VALUE_LAYOUTS {
        assert!(layout.size > 0, "zero-size layout: {}", layout.identity);
        assert!(
            layout.alignment.is_power_of_two(),
            "invalid alignment for {}",
            layout.identity
        );
        assert_eq!(
            layout.size % u64::from(layout.alignment),
            0,
            "unaligned size for {}",
            layout.identity
        );
        if !layout.allocator.is_empty() {
            assert_eq!(layout.direction.ends_with("owned-output"), true);
            assert!(!layout.free_operation.is_empty());
        }
    }
}

#[test]
fn generated_rust_hash_matches_committed_contract() {
    let contract = include_str!("../../../../api/contract.json");
    assert!(
        contract.contains(&format!("\"abiLayoutHash\": \"{ABI_LAYOUT_FINGERPRINT}\"")),
        "generated Rust values and contract hash differ"
    );
    assert!(
        contract.contains(&format!("\"witHash\": \"{WIT_HASH}\"")),
        "generated Rust dispatch and contract WIT hash differ"
    );
}

#[test]
fn owned_nested_values_preserve_unicode_and_empty_lists() {
    let expected = Sample {
        text: "Gate 🧊".to_owned(),
        factor: -7,
        tags: vec!["日本語".to_owned(), String::new()],
    };
    let owned = OwnedSample::from_sample(expected.clone());

    // SAFETY: OwnedSample::from_sample keeps all Rust allocations live until
    // free_rust below. The test only reads the declared lengths.
    unsafe {
        let text = std::slice::from_raw_parts(owned.text.ptr, owned.text.len);
        assert_eq!(text, expected.text.as_bytes());
        assert_eq!(owned.factor, expected.factor);
        let tags = std::slice::from_raw_parts(owned.tags.ptr, owned.tags.len);
        assert_eq!(tags.len(), expected.tags.len());
        for (actual, expected) in tags.iter().zip(&expected.tags) {
            let bytes = if actual.len == 0 {
                &[][..]
            } else {
                std::slice::from_raw_parts(actual.ptr, actual.len)
            };
            assert_eq!(bytes, expected.as_bytes());
        }
        owned.free_rust();
    }
}

struct NoopDispatch;

impl Dispatch for NoopDispatch {
    fn invoke(
        _store: StoreContextMut<'_, Self>,
        operation: &'static Operation,
        _function_type: ComponentFunc,
        _parameters: &[Val],
        _results: &mut [Val],
    ) -> wasmtime::Result<()> {
        Err(wasmtime::Error::msg(format!(
            "unexpected invocation of {}",
            operation.identity
        )))
    }

    fn register_callback(
        _store: StoreContextMut<'_, Self>,
        callback: &'static CallbackDescriptor,
        _guest_id: u64,
        _results: &mut [Val],
    ) -> wasmtime::Result<()> {
        Err(wasmtime::Error::msg(format!(
            "unexpected callback registration of {}",
            callback.identity
        )))
    }

    fn call_callback(
        _store: StoreContextMut<'_, Self>,
        callback: &'static CallbackDescriptor,
        _function_type: ComponentFunc,
        _parameters: &[Val],
        _results: &mut [Val],
    ) -> wasmtime::Result<()> {
        Err(wasmtime::Error::msg(format!(
            "unexpected callback invocation of {}",
            callback.identity
        )))
    }

    fn drop_resource(
        _store: StoreContextMut<'_, Self>,
        _resource: &'static ResourceDescriptor,
        _representation: u32,
    ) -> wasmtime::Result<()> {
        Ok(())
    }
}

#[test]
fn generated_adapters_register_with_wasmtime_linker() {
    let engine = wasmtime::Engine::default();
    let mut linker = Linker::<NoopDispatch>::new(&engine);
    add_to_linker(&mut linker).expect("all generated adapters must register");
}
