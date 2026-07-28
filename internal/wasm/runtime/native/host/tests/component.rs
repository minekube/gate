use std::sync::Arc;
use std::sync::Mutex;
use std::sync::atomic::{AtomicUsize, Ordering};

use gate_wasm_native::{ActiveCall, Engine, Host, Limits, Sample};

const COMPONENT: &[u8] = include_bytes!("../../artifacts/gate_wasm_spike.component.wasm");

struct TestHost;

impl Host for TestHost {
    fn context_is_cancelled(&self, context: u64) -> anyhow::Result<bool> {
        assert_eq!(context, 1);
        Ok(false)
    }

    fn proxy_transform(
        &self,
        proxy: u64,
        mut input: Sample,
    ) -> anyhow::Result<Result<Sample, String>> {
        assert_eq!(proxy, 2);
        input.text = format!("host:{}", input.text);
        input.factor *= 3;
        input.tags.push("host".into());
        Ok(Ok(input))
    }

    fn proxy_emit_nested(
        &self,
        _active: &mut ActiveCall<'_>,
        _proxy: u64,
        _input: String,
    ) -> anyhow::Result<Result<String, String>> {
        unreachable!("nested calls are tested separately")
    }
}

#[test]
fn init_crosses_values_and_resources() -> anyhow::Result<()> {
    let mut engine = Engine::new(COMPONENT, Arc::new(TestHost), Limits::default())?;

    let output = engine.init(1, 2)?;

    assert_eq!(
        output,
        Sample {
            text: "host:init".into(),
            factor: 6,
            tags: vec!["guest".into(), "component".into(), "host".into()],
        }
    );
    Ok(())
}

struct DropHost {
    drops: Arc<AtomicUsize>,
}

impl Drop for DropHost {
    fn drop(&mut self) {
        self.drops.fetch_add(1, Ordering::SeqCst);
    }
}

impl Host for DropHost {
    fn context_is_cancelled(&self, _context: u64) -> anyhow::Result<bool> {
        Ok(false)
    }

    fn proxy_transform(
        &self,
        _proxy: u64,
        input: Sample,
    ) -> anyhow::Result<Result<Sample, String>> {
        Ok(Ok(input))
    }

    fn proxy_emit_nested(
        &self,
        _active: &mut ActiveCall<'_>,
        _proxy: u64,
        _input: String,
    ) -> anyhow::Result<Result<String, String>> {
        unreachable!("nested calls are tested separately")
    }
}

#[test]
fn dropping_engine_releases_host_exactly_once() -> anyhow::Result<()> {
    let drops = Arc::new(AtomicUsize::new(0));
    let host = Arc::new(DropHost {
        drops: Arc::clone(&drops),
    });
    let mut engine = Engine::new(COMPONENT, host.clone(), Limits::default())?;
    engine.init(1, 2)?;
    drop(host);

    assert_eq!(drops.load(Ordering::SeqCst), 0);
    drop(engine);
    assert_eq!(drops.load(Ordering::SeqCst), 1);
    Ok(())
}

struct NestedHost {
    calls: Arc<Mutex<Vec<String>>>,
}

impl Host for NestedHost {
    fn context_is_cancelled(&self, _context: u64) -> anyhow::Result<bool> {
        Ok(false)
    }

    fn proxy_transform(
        &self,
        _proxy: u64,
        input: Sample,
    ) -> anyhow::Result<Result<Sample, String>> {
        if let Some(event) = input.text.strip_prefix("event:") {
            self.calls.lock().unwrap().push(format!("guest:{event}"));
        }
        Ok(Ok(input))
    }

    fn proxy_emit_nested(
        &self,
        active: &mut ActiveCall<'_>,
        proxy: u64,
        input: String,
    ) -> anyhow::Result<Result<String, String>> {
        self.calls.lock().unwrap().push("host:emit-nested".into());
        let result = active.on_event(proxy, &input)?;
        self.calls.lock().unwrap().push("host:return-nested".into());
        Ok(Ok(result))
    }
}

#[test]
fn nested_event_reenters_same_component() -> anyhow::Result<()> {
    let calls = Arc::new(Mutex::new(Vec::new()));
    let host = Arc::new(NestedHost {
        calls: Arc::clone(&calls),
    });
    let mut engine = Engine::new(COMPONENT, host, Limits::default())?;

    let output = engine.on_event(2, "outer")?;

    assert_eq!(output, "outer:guest:inner");
    assert_eq!(
        *calls.lock().unwrap(),
        [
            "guest:outer",
            "host:emit-nested",
            "guest:inner",
            "host:return-nested",
        ]
    );
    Ok(())
}

#[test]
fn nested_guest_trap_traps_outer_invocation() -> anyhow::Result<()> {
    let calls = Arc::new(Mutex::new(Vec::new()));
    let host = Arc::new(NestedHost {
        calls: Arc::clone(&calls),
    });
    let mut engine = Engine::new(COMPONENT, host, Limits::default())?;

    let error = engine.on_event(2, "outer-trap").unwrap_err();

    assert!(
        format!("{error:#}").contains("component event callback trapped"),
        "unexpected error: {error:#}"
    );
    assert_eq!(
        *calls.lock().unwrap(),
        ["guest:outer-trap", "host:emit-nested", "guest:trap"]
    );
    Ok(())
}
