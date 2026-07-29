use gate_wasm_native::wire::{
    GateError, Response, WireValue, decode_request, decode_response, encode_request,
    encode_response,
};

#[test]
fn nested_language_neutral_values_round_trip() -> anyhow::Result<()> {
    let values = vec![
        WireValue::Bool(true),
        WireValue::S64(-42),
        WireValue::U64(99),
        WireValue::String("héllo 世界".into()),
        WireValue::List(vec![]),
        WireValue::Record(vec![
            ("name".into(), WireValue::String("gate".into())),
            (
                "items".into(),
                WireValue::List(vec![
                    WireValue::U64(1),
                    WireValue::Null,
                    WireValue::String("three".into()),
                ]),
            ),
        ]),
        WireValue::Resource(0x0102_0304_0506_0708),
        WireValue::Null,
    ];

    let encoded = encode_request(&values)?;
    assert_eq!(decode_request(&encoded)?, values);
    Ok(())
}

#[test]
fn callback_responses_round_trip() -> anyhow::Result<()> {
    let success = Response {
        values: vec![WireValue::Bool(true)],
        error: None,
    };
    assert_eq!(decode_response(&encode_response(&success)?)?, success);

    let failure = Response {
        values: vec![],
        error: Some(GateError {
            kind: "guest-error".into(),
            message: "failed".into(),
            operation: "callback".into(),
        }),
    };
    assert_eq!(decode_response(&encode_response(&failure)?)?, failure);
    Ok(())
}

#[test]
fn malformed_and_trailing_wire_input_is_rejected() -> anyhow::Result<()> {
    assert!(decode_request(&[1, 1, 14, 3, b'x']).is_err());
    let mut encoded = encode_request(&[WireValue::String("ok".into())])?;
    encoded.push(0);
    assert!(decode_request(&encoded).is_err());
    Ok(())
}
