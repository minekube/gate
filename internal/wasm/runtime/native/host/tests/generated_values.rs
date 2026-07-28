use gate_wasm_native::generated::values::{
    ABI_LAYOUT_FINGERPRINT, ABI_SCHEMA_VERSION, VALUE_LAYOUTS,
};
use gate_wasm_native::{Sample, abi::OwnedSample};

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
