wit_bindgen::generate!({
    path: "../wit",
    world: "gate-plugin",
});

struct Spike;

impl exports::minekube::gate_spike::plugin::Guest for Spike {
    fn init(
        context: &minekube::gate_spike::host::Context,
        proxy: &minekube::gate_spike::host::Proxy,
    ) -> Result<minekube::gate_spike::host::Sample, String> {
        if context.is_cancelled() {
            return Err("context cancelled".into());
        }
        proxy.transform(&minekube::gate_spike::host::Sample {
            text: "init".into(),
            factor: 2,
            tags: vec!["guest".into(), "component".into()],
        })
    }

    fn on_event(
        proxy: &minekube::gate_spike::host::Proxy,
        input: String,
    ) -> Result<String, String> {
        proxy.transform(&minekube::gate_spike::host::Sample {
            text: format!("event:{input}"),
            factor: 1,
            tags: Vec::new(),
        })?;
        if input == "outer" {
            return Ok(format!("outer:{}", proxy.emit_nested("inner")?));
        }
        if input == "outer-trap" {
            return Ok(format!("outer:{}", proxy.emit_nested("trap")?));
        }
        if input == "trap" {
            panic!("nested guest trap");
        }
        if input == "large" {
            return Ok("x".repeat(1024));
        }
        Ok(format!("guest:{input}"))
    }

    fn allocate(bytes: u64) -> u64 {
        let allocation = vec![0_u8; usize::try_from(bytes).unwrap()];
        std::hint::black_box(&allocation);
        allocation.len() as u64
    }

    fn spin() {
        loop {
            core::hint::spin_loop();
        }
    }
}

export!(Spike);
