use std::env;
use std::fs;
use std::path::PathBuf;

fn main() {
    let contract = PathBuf::from("../../generated/contract.json");
    println!("cargo:rerun-if-changed={}", contract.display());
    let document = fs::read_to_string(&contract).expect("read generated Gate contract");
    let hash = string_field(&document, "witHash");
    let format = integer_field(&document, "generatorFormat");
    let generated = format!(
        "const GATE_WIT_HASH: &str = {hash:?};\nconst GATE_GENERATOR_FORMAT: u32 = {format};\n"
    );
    let output = PathBuf::from(env::var_os("OUT_DIR").expect("OUT_DIR")).join("gate_contract.rs");
    fs::write(output, generated).expect("write generated contract constants");
}

fn string_field<'a>(document: &'a str, name: &str) -> &'a str {
    let marker = format!("\"{name}\": \"");
    document
        .split_once(&marker)
        .and_then(|(_, value)| value.split_once('"'))
        .map(|(value, _)| value)
        .unwrap_or_else(|| panic!("missing string field {name}"))
}

fn integer_field(document: &str, name: &str) -> u32 {
    let marker = format!("\"{name}\": ");
    let value = document
        .split_once(&marker)
        .map(|(_, value)| value)
        .and_then(|value| value.split([',', '\n']).next())
        .unwrap_or_else(|| panic!("missing integer field {name}"));
    value
        .parse()
        .unwrap_or_else(|_| panic!("invalid integer field {name}"))
}
