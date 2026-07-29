use anyhow::{Context as _, bail};
use wasmtime::StoreContextMut;
use wasmtime::component::{
    ResourceDynamic, ResourceType, Val,
    types::{ComponentFunc, Type},
};

use crate::generated::dispatch::RESOURCE_TYPES;

pub const VERSION: u8 = 1;
const MAX_COLLECTION_ELEMENTS: usize = 1 << 24;

const TAG_NULL: u8 = 0;
const TAG_FALSE: u8 = 1;
const TAG_TRUE: u8 = 2;
const TAG_S8: u8 = 3;
const TAG_U8: u8 = 4;
const TAG_S16: u8 = 5;
const TAG_U16: u8 = 6;
const TAG_S32: u8 = 7;
const TAG_U32: u8 = 8;
const TAG_S64: u8 = 9;
const TAG_U64: u8 = 10;
const TAG_F32: u8 = 11;
const TAG_F64: u8 = 12;
const TAG_CHAR: u8 = 13;
const TAG_STRING: u8 = 14;
const TAG_LIST: u8 = 15;
const TAG_MAP: u8 = 16;
const TAG_RECORD: u8 = 17;
const TAG_TUPLE: u8 = 18;
const TAG_VARIANT: u8 = 19;
const TAG_ENUM: u8 = 20;
const TAG_FLAGS: u8 = 21;
const TAG_RESOURCE: u8 = 22;
const TAG_RESULT: u8 = 23;

#[derive(Clone, Debug, PartialEq)]
pub enum WireValue {
    Null,
    Bool(bool),
    S8(i8),
    U8(u8),
    S16(i16),
    U16(u16),
    S32(i32),
    U32(u32),
    S64(i64),
    U64(u64),
    F32(f32),
    F64(f64),
    Char(char),
    String(String),
    List(Vec<WireValue>),
    Map(Vec<(WireValue, WireValue)>),
    Record(Vec<(String, WireValue)>),
    Tuple(Vec<WireValue>),
    Variant {
        name: String,
        value: Option<Box<WireValue>>,
    },
    Enum(String),
    Flags(Vec<String>),
    Resource(u64),
    Result {
        error: bool,
        value: Option<Box<WireValue>>,
    },
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct GateError {
    pub kind: String,
    pub message: String,
    pub operation: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct Response {
    pub values: Vec<WireValue>,
    pub error: Option<GateError>,
}

pub trait ResourceTransport {
    fn resource_handle(&self, representation: u32) -> anyhow::Result<u64>;
    fn insert_resource(&mut self, handle: u64, type_id: u32, owned: bool) -> anyhow::Result<u32>;
}

pub fn encode_parameters<T: ResourceTransport>(
    mut store: StoreContextMut<'_, T>,
    values: &[Val],
) -> anyhow::Result<Vec<u8>> {
    let mut encoded = Vec::with_capacity(values.len());
    for value in values {
        encoded.push(value_to_wire(&mut store, value)?);
    }
    encode_request(&encoded)
}

pub fn decode_results<T: ResourceTransport>(
    mut store: StoreContextMut<'_, T>,
    function_type: &ComponentFunc,
    response: &[u8],
    results: &mut [Val],
) -> anyhow::Result<()> {
    let response = decode_response(response)?;
    let result_types: Vec<_> = function_type.results().collect();
    if result_types.len() != results.len() {
        bail!(
            "host result slot mismatch: function has {}, Wasmtime supplied {}",
            result_types.len(),
            results.len()
        );
    }
    match result_types.as_slice() {
        [] => {
            if let Some(error) = response.error {
                bail!("{}: {} ({})", error.operation, error.message, error.kind);
            }
            if !response.values.is_empty() {
                bail!(
                    "Go returned {} values for a function with no result",
                    response.values.len()
                );
            }
        }
        [Type::Result(result_type)] => {
            let value = if let Some(error) = response.error {
                let error_type = result_type
                    .err()
                    .context("operation returned an error but WIT result has no error type")?;
                let error = WireValue::Record(vec![
                    ("kind".into(), WireValue::String(error.kind)),
                    ("message".into(), WireValue::String(error.message)),
                    ("operation".into(), WireValue::String(error.operation)),
                ]);
                Val::Result(Err(Some(Box::new(wire_to_value(
                    &mut store,
                    &error_type,
                    error,
                )?))))
            } else {
                let payload = result_type
                    .ok()
                    .map(|expected| {
                        response_payload(response.values)
                            .and_then(|value| wire_to_value(&mut store, &expected, value))
                            .map(Box::new)
                    })
                    .transpose()?;
                Val::Result(Ok(payload))
            };
            results[0] = value;
        }
        [expected] => {
            if let Some(error) = response.error {
                bail!("{}: {} ({})", error.operation, error.message, error.kind);
            }
            let value = response_payload(response.values)?;
            results[0] = wire_to_value(&mut store, expected, value)?;
        }
        _ => bail!(
            "unsupported component function with {} flat results",
            result_types.len()
        ),
    }
    Ok(())
}

fn response_payload(mut values: Vec<WireValue>) -> anyhow::Result<WireValue> {
    match values.len() {
        0 => bail!("Go returned no value for a result payload"),
        1 => Ok(values.pop().expect("one response value")),
        _ => Ok(WireValue::Tuple(values)),
    }
}

fn value_to_wire<T: ResourceTransport>(
    store: &mut StoreContextMut<'_, T>,
    value: &Val,
) -> anyhow::Result<WireValue> {
    Ok(match value {
        Val::Bool(value) => WireValue::Bool(*value),
        Val::S8(value) => WireValue::S8(*value),
        Val::U8(value) => WireValue::U8(*value),
        Val::S16(value) => WireValue::S16(*value),
        Val::U16(value) => WireValue::U16(*value),
        Val::S32(value) => WireValue::S32(*value),
        Val::U32(value) => WireValue::U32(*value),
        Val::S64(value) => WireValue::S64(*value),
        Val::U64(value) => WireValue::U64(*value),
        Val::Float32(value) => WireValue::F32(*value),
        Val::Float64(value) => WireValue::F64(*value),
        Val::Char(value) => WireValue::Char(*value),
        Val::String(value) => WireValue::String(value.clone()),
        Val::List(values) => WireValue::List(
            values
                .iter()
                .map(|value| value_to_wire(store, value))
                .collect::<anyhow::Result<_>>()?,
        ),
        Val::Map(entries) => WireValue::Map(
            entries
                .iter()
                .map(|(key, value)| Ok((value_to_wire(store, key)?, value_to_wire(store, value)?)))
                .collect::<anyhow::Result<_>>()?,
        ),
        Val::Record(fields) => WireValue::Record(
            fields
                .iter()
                .map(|(name, value)| Ok((name.clone(), value_to_wire(store, value)?)))
                .collect::<anyhow::Result<_>>()?,
        ),
        Val::Tuple(values) => WireValue::Tuple(
            values
                .iter()
                .map(|value| value_to_wire(store, value))
                .collect::<anyhow::Result<_>>()?,
        ),
        Val::Variant(name, value) => WireValue::Variant {
            name: name.clone(),
            value: value
                .as_deref()
                .map(|value| value_to_wire(store, value).map(Box::new))
                .transpose()?,
        },
        Val::Enum(value) => WireValue::Enum(value.clone()),
        Val::Option(value) => match value {
            Some(value) => value_to_wire(store, value)?,
            None => WireValue::Null,
        },
        Val::Result(value) => match value {
            Ok(value) => WireValue::Result {
                error: false,
                value: value
                    .as_deref()
                    .map(|value| value_to_wire(store, value).map(Box::new))
                    .transpose()?,
            },
            Err(value) => WireValue::Result {
                error: true,
                value: value
                    .as_deref()
                    .map(|value| value_to_wire(store, value).map(Box::new))
                    .transpose()?,
            },
        },
        Val::Flags(values) => WireValue::Flags(values.clone()),
        Val::Resource(value) => {
            let resource = value.clone().try_into_resource_dynamic(&mut *store)?;
            WireValue::Resource(store.data().resource_handle(resource.rep())?)
        }
        Val::Future(_) | Val::Stream(_) | Val::ErrorContext(_) => {
            bail!("unsupported asynchronous component value")
        }
    })
}

fn wire_to_value<T: ResourceTransport>(
    store: &mut StoreContextMut<'_, T>,
    expected: &Type,
    value: WireValue,
) -> anyhow::Result<Val> {
    Ok(match expected {
        Type::Bool => Val::Bool(expect_bool(value)?),
        Type::S8 => Val::S8(expect_signed(value)?.try_into()?),
        Type::U8 => Val::U8(expect_unsigned(value)?.try_into()?),
        Type::S16 => Val::S16(expect_signed(value)?.try_into()?),
        Type::U16 => Val::U16(expect_unsigned(value)?.try_into()?),
        Type::S32 => Val::S32(expect_signed(value)?.try_into()?),
        Type::U32 => Val::U32(expect_unsigned(value)?.try_into()?),
        Type::S64 => Val::S64(expect_signed(value)?),
        Type::U64 => Val::U64(expect_unsigned(value)?),
        Type::Float32 => Val::Float32(expect_float(value)? as f32),
        Type::Float64 => Val::Float64(expect_float(value)?),
        Type::Char => match value {
            WireValue::Char(value) => Val::Char(value),
            other => bail!("expected char, got {other:?}"),
        },
        Type::String => match value {
            WireValue::String(value) => Val::String(value),
            other => bail!("expected string, got {other:?}"),
        },
        Type::List(list_type) => {
            let values = match value {
                WireValue::List(values) => values,
                other => bail!("expected list, got {other:?}"),
            };
            Val::List(
                values
                    .into_iter()
                    .map(|value| wire_to_value(store, &list_type.ty(), value))
                    .collect::<anyhow::Result<_>>()?,
            )
        }
        Type::Map(map_type) => {
            let values = match value {
                WireValue::Map(values) => values,
                other => bail!("expected map, got {other:?}"),
            };
            Val::Map(
                values
                    .into_iter()
                    .map(|(key, value)| {
                        Ok((
                            wire_to_value(store, &map_type.key(), key)?,
                            wire_to_value(store, &map_type.value(), value)?,
                        ))
                    })
                    .collect::<anyhow::Result<_>>()?,
            )
        }
        Type::Record(record_type) => {
            let values = match value {
                WireValue::Record(values) => values,
                other => bail!("expected record, got {other:?}"),
            };
            let mut values = std::collections::BTreeMap::from_iter(values);
            Val::Record(
                record_type
                    .fields()
                    .map(|field| {
                        let value = values
                            .remove(field.name)
                            .with_context(|| format!("record field {} is missing", field.name))?;
                        Ok((
                            field.name.to_owned(),
                            wire_to_value(store, &field.ty, value)?,
                        ))
                    })
                    .collect::<anyhow::Result<_>>()?,
            )
        }
        Type::Tuple(tuple_type) => {
            let values = match value {
                WireValue::Tuple(values) | WireValue::List(values) => values,
                other => bail!("expected tuple, got {other:?}"),
            };
            let types: Vec<_> = tuple_type.types().collect();
            if values.len() != types.len() {
                bail!("tuple has {} items, expected {}", values.len(), types.len());
            }
            Val::Tuple(
                types
                    .iter()
                    .zip(values)
                    .map(|(expected, value)| wire_to_value(store, expected, value))
                    .collect::<anyhow::Result<_>>()?,
            )
        }
        Type::Variant(variant_type) => {
            let (name, value) = match value {
                WireValue::Variant { name, value } => (name, value),
                other => bail!("expected variant, got {other:?}"),
            };
            let case = variant_type
                .cases()
                .find(|case| case.name == name)
                .with_context(|| format!("unknown variant case {name}"))?;
            let value = match (case.ty, value) {
                (Some(expected), Some(value)) => {
                    Some(Box::new(wire_to_value(store, &expected, *value)?))
                }
                (None, None) => None,
                _ => bail!("variant case {name} payload mismatch"),
            };
            Val::Variant(name, value)
        }
        Type::Enum(enum_type) => {
            let name = match value {
                WireValue::Enum(name) | WireValue::String(name) => name,
                other => bail!("expected enum, got {other:?}"),
            };
            if !enum_type.names().any(|candidate| candidate == name) {
                bail!("unknown enum case {name}");
            }
            Val::Enum(name)
        }
        Type::Option(option_type) => match value {
            WireValue::Null => Val::Option(None),
            value => Val::Option(Some(Box::new(wire_to_value(
                store,
                &option_type.ty(),
                value,
            )?))),
        },
        Type::Result(result_type) => {
            let (error, value) = match value {
                WireValue::Result { error, value } => (error, value),
                other => bail!("expected result, got {other:?}"),
            };
            let expected = if error {
                result_type.err()
            } else {
                result_type.ok()
            };
            let value = match (expected, value) {
                (Some(expected), Some(value)) => {
                    Some(Box::new(wire_to_value(store, &expected, *value)?))
                }
                (None, None) => None,
                _ => bail!("result payload mismatch"),
            };
            Val::Result(if error { Err(value) } else { Ok(value) })
        }
        Type::Flags(flags_type) => {
            let values = match value {
                WireValue::Flags(values) => values,
                other => bail!("expected flags, got {other:?}"),
            };
            let known: std::collections::HashSet<_> = flags_type.names().collect();
            if let Some(unknown) = values.iter().find(|value| !known.contains(value.as_str())) {
                bail!("unknown flag {unknown}");
            }
            Val::Flags(values)
        }
        Type::Own(resource_type) | Type::Borrow(resource_type) => {
            let handle = match value {
                WireValue::Resource(handle) => handle,
                other => bail!("expected resource, got {other:?}"),
            };
            let type_id = resource_type_id(*resource_type)?;
            let owned = matches!(expected, Type::Own(_));
            let representation = store.data_mut().insert_resource(handle, type_id, owned)?;
            let resource = if owned {
                ResourceDynamic::new_own(representation, type_id)
            } else {
                ResourceDynamic::new_borrow(representation, type_id)
            };
            Val::Resource(resource.try_into_resource_any(&mut *store)?)
        }
        Type::Future(_) | Type::Stream(_) | Type::ErrorContext => {
            bail!("unsupported asynchronous component result type")
        }
    })
}

fn resource_type_id(resource_type: ResourceType) -> anyhow::Result<u32> {
    RESOURCE_TYPES
        .iter()
        .find(|resource| ResourceType::host_dynamic(resource.id) == resource_type)
        .map(|resource| resource.id)
        .context("component resource type is not registered by Gate")
}

fn expect_bool(value: WireValue) -> anyhow::Result<bool> {
    match value {
        WireValue::Bool(value) => Ok(value),
        other => bail!("expected bool, got {other:?}"),
    }
}

fn expect_signed(value: WireValue) -> anyhow::Result<i64> {
    match value {
        WireValue::S8(value) => Ok(i64::from(value)),
        WireValue::U8(value) => Ok(i64::from(value)),
        WireValue::S16(value) => Ok(i64::from(value)),
        WireValue::U16(value) => Ok(i64::from(value)),
        WireValue::S32(value) => Ok(i64::from(value)),
        WireValue::U32(value) => Ok(i64::from(value)),
        WireValue::S64(value) => Ok(value),
        WireValue::U64(value) => Ok(value.try_into()?),
        other => bail!("expected integer, got {other:?}"),
    }
}

fn expect_unsigned(value: WireValue) -> anyhow::Result<u64> {
    match value {
        WireValue::S8(value) => Ok(value.try_into()?),
        WireValue::U8(value) => Ok(u64::from(value)),
        WireValue::S16(value) => Ok(value.try_into()?),
        WireValue::U16(value) => Ok(u64::from(value)),
        WireValue::S32(value) => Ok(value.try_into()?),
        WireValue::U32(value) => Ok(u64::from(value)),
        WireValue::S64(value) => Ok(value.try_into()?),
        WireValue::U64(value) => Ok(value),
        other => bail!("expected integer, got {other:?}"),
    }
}

fn expect_float(value: WireValue) -> anyhow::Result<f64> {
    match value {
        WireValue::F32(value) => Ok(f64::from(value)),
        WireValue::F64(value) => Ok(value),
        other => bail!("expected float, got {other:?}"),
    }
}

pub fn encode_request(values: &[WireValue]) -> anyhow::Result<Vec<u8>> {
    let mut output = vec![VERSION];
    write_uint(&mut output, values.len() as u64);
    for value in values {
        encode_value(&mut output, value)?;
    }
    Ok(output)
}

pub fn decode_request(input: &[u8]) -> anyhow::Result<Vec<WireValue>> {
    let mut reader = Reader::new(input);
    reader.version()?;
    let count = reader.count("value")?;
    let mut values = Vec::with_capacity(count);
    for _ in 0..count {
        values.push(reader.value()?);
    }
    reader.finish("wire input")?;
    Ok(values)
}

pub fn decode_response(input: &[u8]) -> anyhow::Result<Response> {
    let mut reader = Reader::new(input);
    reader.version()?;
    let status = reader.byte().context("wire response status is missing")?;
    let response = match status {
        0 => {
            let count = reader.count("response value")?;
            let mut values = Vec::with_capacity(count);
            for _ in 0..count {
                values.push(reader.value()?);
            }
            Response {
                values,
                error: None,
            }
        }
        1 => Response {
            values: Vec::new(),
            error: Some(GateError {
                kind: reader.string().context("response error kind")?,
                message: reader.string().context("response error message")?,
                operation: reader.string().context("response error operation")?,
            }),
        },
        other => bail!("unknown wire response status {other}"),
    };
    reader.finish("wire response")?;
    Ok(response)
}

fn encode_value(output: &mut Vec<u8>, value: &WireValue) -> anyhow::Result<()> {
    match value {
        WireValue::Null => output.push(TAG_NULL),
        WireValue::Bool(false) => output.push(TAG_FALSE),
        WireValue::Bool(true) => output.push(TAG_TRUE),
        WireValue::S8(value) => {
            output.push(TAG_S8);
            output.push(*value as u8);
        }
        WireValue::U8(value) => {
            output.push(TAG_U8);
            output.push(*value);
        }
        WireValue::S16(value) => {
            output.push(TAG_S16);
            output.extend_from_slice(&value.to_le_bytes());
        }
        WireValue::U16(value) => {
            output.push(TAG_U16);
            output.extend_from_slice(&value.to_le_bytes());
        }
        WireValue::S32(value) => {
            output.push(TAG_S32);
            output.extend_from_slice(&value.to_le_bytes());
        }
        WireValue::U32(value) => {
            output.push(TAG_U32);
            output.extend_from_slice(&value.to_le_bytes());
        }
        WireValue::S64(value) => {
            output.push(TAG_S64);
            output.extend_from_slice(&value.to_le_bytes());
        }
        WireValue::U64(value) => {
            output.push(TAG_U64);
            output.extend_from_slice(&value.to_le_bytes());
        }
        WireValue::F32(value) => {
            output.push(TAG_F32);
            output.extend_from_slice(&value.to_bits().to_le_bytes());
        }
        WireValue::F64(value) => {
            output.push(TAG_F64);
            output.extend_from_slice(&value.to_bits().to_le_bytes());
        }
        WireValue::Char(value) => {
            output.push(TAG_CHAR);
            output.extend_from_slice(&u32::from(*value).to_le_bytes());
        }
        WireValue::String(value) => {
            output.push(TAG_STRING);
            write_string(output, value);
        }
        WireValue::List(values) => {
            output.push(TAG_LIST);
            encode_sequence(output, values)?;
        }
        WireValue::Map(entries) => {
            output.push(TAG_MAP);
            write_uint(output, entries.len() as u64);
            for (key, value) in entries {
                encode_value(output, key)?;
                encode_value(output, value)?;
            }
        }
        WireValue::Record(fields) => {
            output.push(TAG_RECORD);
            write_uint(output, fields.len() as u64);
            for (name, value) in fields {
                write_string(output, name);
                encode_value(output, value)?;
            }
        }
        WireValue::Tuple(values) => {
            output.push(TAG_TUPLE);
            encode_sequence(output, values)?;
        }
        WireValue::Variant { name, value } => {
            output.push(TAG_VARIANT);
            write_string(output, name);
            output.push(u8::from(value.is_some()));
            if let Some(value) = value {
                encode_value(output, value)?;
            }
        }
        WireValue::Enum(value) => {
            output.push(TAG_ENUM);
            write_string(output, value);
        }
        WireValue::Flags(values) => {
            output.push(TAG_FLAGS);
            write_uint(output, values.len() as u64);
            for value in values {
                write_string(output, value);
            }
        }
        WireValue::Resource(value) => {
            output.push(TAG_RESOURCE);
            output.extend_from_slice(&value.to_le_bytes());
        }
        WireValue::Result { error, value } => {
            output.push(TAG_RESULT);
            output.push(u8::from(*error) | (u8::from(value.is_some()) << 1));
            if let Some(value) = value {
                encode_value(output, value)?;
            }
        }
    }
    Ok(())
}

fn encode_sequence(output: &mut Vec<u8>, values: &[WireValue]) -> anyhow::Result<()> {
    write_uint(output, values.len() as u64);
    for value in values {
        encode_value(output, value)?;
    }
    Ok(())
}

fn write_string(output: &mut Vec<u8>, value: &str) {
    write_uint(output, value.len() as u64);
    output.extend_from_slice(value.as_bytes());
}

fn write_uint(output: &mut Vec<u8>, mut value: u64) {
    while value >= 0x80 {
        output.push((value as u8) | 0x80);
        value >>= 7;
    }
    output.push(value as u8);
}

struct Reader<'a> {
    input: &'a [u8],
    offset: usize,
}

impl<'a> Reader<'a> {
    fn new(input: &'a [u8]) -> Self {
        Self { input, offset: 0 }
    }

    fn version(&mut self) -> anyhow::Result<()> {
        let version = self.byte().context("wire version is missing")?;
        if version != VERSION {
            bail!("unsupported wire version {version}");
        }
        Ok(())
    }

    fn finish(&self, kind: &str) -> anyhow::Result<()> {
        if self.offset != self.input.len() {
            bail!(
                "{kind} has {} trailing bytes",
                self.input.len() - self.offset
            );
        }
        Ok(())
    }

    fn byte(&mut self) -> Option<u8> {
        let value = *self.input.get(self.offset)?;
        self.offset += 1;
        Some(value)
    }

    fn fixed<const N: usize>(&mut self) -> anyhow::Result<[u8; N]> {
        let end = self
            .offset
            .checked_add(N)
            .context("wire offset overflowed")?;
        let bytes = self
            .input
            .get(self.offset..end)
            .context("wire value is truncated")?;
        self.offset = end;
        Ok(bytes.try_into().expect("slice has fixed size"))
    }

    fn uint(&mut self) -> anyhow::Result<u64> {
        let mut value = 0_u64;
        for shift in (0..=63).step_by(7) {
            let byte = self.byte().context("wire integer is truncated")?;
            if shift == 63 && byte > 1 {
                bail!("wire integer overflowed");
            }
            value |= u64::from(byte & 0x7f) << shift;
            if byte & 0x80 == 0 {
                return Ok(value);
            }
        }
        bail!("wire integer overflowed")
    }

    fn count(&mut self, kind: &str) -> anyhow::Result<usize> {
        let count = usize::try_from(self.uint()?).context("wire count does not fit usize")?;
        if count > MAX_COLLECTION_ELEMENTS {
            bail!("{kind} has too many elements: {count}");
        }
        Ok(count)
    }

    fn string(&mut self) -> anyhow::Result<String> {
        let length = usize::try_from(self.uint()?).context("string length does not fit usize")?;
        let end = self
            .offset
            .checked_add(length)
            .context("string length overflowed")?;
        let bytes = self
            .input
            .get(self.offset..end)
            .context("string exceeds remaining wire bytes")?;
        self.offset = end;
        Ok(std::str::from_utf8(bytes)
            .context("wire string is not UTF-8")?
            .to_owned())
    }

    fn value(&mut self) -> anyhow::Result<WireValue> {
        let tag = self.byte().context("wire value tag is missing")?;
        Ok(match tag {
            TAG_NULL => WireValue::Null,
            TAG_FALSE => WireValue::Bool(false),
            TAG_TRUE => WireValue::Bool(true),
            TAG_S8 => WireValue::S8(self.byte().context("s8 is truncated")? as i8),
            TAG_U8 => WireValue::U8(self.byte().context("u8 is truncated")?),
            TAG_S16 => WireValue::S16(i16::from_le_bytes(self.fixed()?)),
            TAG_U16 => WireValue::U16(u16::from_le_bytes(self.fixed()?)),
            TAG_S32 => WireValue::S32(i32::from_le_bytes(self.fixed()?)),
            TAG_U32 => WireValue::U32(u32::from_le_bytes(self.fixed()?)),
            TAG_S64 => WireValue::S64(i64::from_le_bytes(self.fixed()?)),
            TAG_U64 => WireValue::U64(u64::from_le_bytes(self.fixed()?)),
            TAG_F32 => WireValue::F32(f32::from_bits(u32::from_le_bytes(self.fixed()?))),
            TAG_F64 => WireValue::F64(f64::from_bits(u64::from_le_bytes(self.fixed()?))),
            TAG_CHAR => {
                let value = u32::from_le_bytes(self.fixed()?);
                WireValue::Char(char::from_u32(value).context("wire character is invalid")?)
            }
            TAG_STRING => WireValue::String(self.string()?),
            TAG_LIST => WireValue::List(self.sequence()?),
            TAG_MAP => {
                let count = self.count("map")?;
                let mut entries = Vec::with_capacity(count);
                for _ in 0..count {
                    entries.push((self.value()?, self.value()?));
                }
                WireValue::Map(entries)
            }
            TAG_RECORD => {
                let count = self.count("record")?;
                let mut fields = Vec::with_capacity(count);
                for _ in 0..count {
                    fields.push((self.string()?, self.value()?));
                }
                WireValue::Record(fields)
            }
            TAG_TUPLE => WireValue::Tuple(self.sequence()?),
            TAG_VARIANT => {
                let name = self.string()?;
                let value = match self.byte().context("variant payload flag is missing")? {
                    0 => None,
                    1 => Some(Box::new(self.value()?)),
                    other => bail!("invalid variant payload flag {other}"),
                };
                WireValue::Variant { name, value }
            }
            TAG_ENUM => WireValue::Enum(self.string()?),
            TAG_FLAGS => {
                let count = self.count("flags")?;
                let mut values = Vec::with_capacity(count);
                for _ in 0..count {
                    values.push(self.string()?);
                }
                WireValue::Flags(values)
            }
            TAG_RESOURCE => WireValue::Resource(u64::from_le_bytes(self.fixed()?)),
            TAG_RESULT => {
                let discriminant = self.byte().context("result discriminant is missing")?;
                if discriminant & !3 != 0 {
                    bail!("invalid result discriminant {discriminant}");
                }
                WireValue::Result {
                    error: discriminant & 1 != 0,
                    value: if discriminant & 2 != 0 {
                        Some(Box::new(self.value()?))
                    } else {
                        None
                    },
                }
            }
            other => bail!("unknown wire value tag {other}"),
        })
    }

    fn sequence(&mut self) -> anyhow::Result<Vec<WireValue>> {
        let count = self.count("list")?;
        let mut values = Vec::with_capacity(count);
        for _ in 0..count {
            values.push(self.value()?);
        }
        Ok(values)
    }
}
