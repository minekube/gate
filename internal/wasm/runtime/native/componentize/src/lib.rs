use anyhow::Context;
use std::ffi::OsString;
use std::path::Path;
use std::path::PathBuf;

pub fn encode(module: &[u8]) -> anyhow::Result<Vec<u8>> {
    wit_component::ComponentEncoder::default()
        .module(module)
        .context("failed to decode core module")?
        .validate(true)
        .encode()
        .context("failed to encode component")
}

pub fn validate_wit(path: impl AsRef<Path>) -> anyhow::Result<()> {
    let path = path.as_ref();
    let mut resolve = wit_parser::Resolve::default();
    resolve
        .push_path(path)
        .with_context(|| format!("failed to validate WIT {}", path.display()))?;
    Ok(())
}

pub fn run<I, S>(args: I) -> anyhow::Result<()>
where
    I: IntoIterator<Item = S>,
    S: Into<OsString>,
{
    let mut args = args.into_iter().map(Into::into);
    let Some(input) = args.next() else {
        anyhow::bail!("usage: gate-wasm-componentize <core-module.wasm> <component.wasm>");
    };
    let Some(output) = args.next() else {
        anyhow::bail!("usage: gate-wasm-componentize <core-module.wasm> <component.wasm>");
    };
    if args.next().is_some() {
        anyhow::bail!("usage: gate-wasm-componentize <core-module.wasm> <component.wasm>");
    }

    let input = PathBuf::from(input);
    let output = PathBuf::from(output);
    let module = std::fs::read(&input)
        .with_context(|| format!("failed to read core module {}", input.display()))?;
    let component = encode(&module)?;
    if let Some(parent) = output.parent() {
        std::fs::create_dir_all(parent)
            .with_context(|| format!("failed to create {}", parent.display()))?;
    }
    std::fs::write(&output, component)
        .with_context(|| format!("failed to write component {}", output.display()))
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::{encode, run, validate_wit};

    #[test]
    fn invalid_core_module_reports_decode_context() {
        let error = encode(b"not wasm").expect_err("invalid core module must fail");

        assert!(
            error.to_string().contains("failed to decode core module"),
            "unexpected error: {error:#}"
        );
    }

    #[test]
    fn command_requires_input_and_output_paths() {
        let error = run(Vec::<String>::new()).expect_err("missing paths must fail");

        assert_eq!(
            error.to_string(),
            "usage: gate-wasm-componentize <core-module.wasm> <component.wasm>"
        );
    }

    #[test]
    fn generated_wit_golden_is_valid() {
        let path = Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../../../generate/testdata/simple.golden.wit");

        validate_wit(&path).unwrap_or_else(|error| {
            panic!("generated WIT {} is invalid: {error:#}", path.display())
        });
    }

    #[test]
    fn generated_gate_wit_is_valid() {
        let path = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../../api/gate.wit");

        validate_wit(&path).unwrap_or_else(|error| {
            panic!("generated WIT {} is invalid: {error:#}", path.display())
        });
    }
}
