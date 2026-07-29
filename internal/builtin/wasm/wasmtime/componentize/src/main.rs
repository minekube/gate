fn main() {
    if let Err(error) = gate_wasm_componentize::run(std::env::args_os().skip(1)) {
        eprintln!("{error:#}");
        std::process::exit(2);
    }
}
