use std::collections::HashMap;
use std::sync::mpsc::{SyncSender, sync_channel};
use std::sync::{Arc, mpsc};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use anyhow::{Context as _, anyhow};
use wasmtime::component::{Access, Component, HasSelf, Instance, Linker, Resource};
use wasmtime::{Config, Store, StoreLimits, StoreLimitsBuilder};

use crate::{ActiveCall, Host as GateHost, Limits, MemoryLimitError, Sample, TransferLimitError};

pub(crate) mod bindings {
    wasmtime::component::bindgen!({
        path: "../wit",
        world: "gate-plugin",
        imports: { default: trappable | store },
    });
}

use bindings::exports::minekube::gate_spike::plugin::Guest;
use bindings::minekube::gate_spike::host::{
    Context, HostContext, HostContextWithStore, HostProxy, HostProxyWithStore, Proxy,
    Sample as WitSample,
};

pub(crate) struct StoreData {
    host: Arc<dyn GateHost>,
    contexts: HashMap<u32, u64>,
    proxies: HashMap<u32, u64>,
    next_resource: u32,
    instance: Option<Instance>,
    store_limits: StoreLimits,
    transfer_bytes: usize,
}

impl StoreData {
    fn insert_context(&mut self, id: u64) -> anyhow::Result<Resource<Context>> {
        let rep = self.next_rep()?;
        self.contexts.insert(rep, id);
        Ok(Resource::new_borrow(rep))
    }

    fn insert_proxy(&mut self, id: u64) -> anyhow::Result<Resource<Proxy>> {
        let rep = self.next_rep()?;
        self.proxies.insert(rep, id);
        Ok(Resource::new_borrow(rep))
    }

    fn next_rep(&mut self) -> anyhow::Result<u32> {
        let rep = self.next_resource;
        self.next_resource = self
            .next_resource
            .checked_add(1)
            .ok_or_else(|| anyhow!("resource representation exhausted"))?;
        Ok(rep)
    }

    fn context_id(&self, resource: &Resource<Context>) -> wasmtime::Result<u64> {
        self.contexts.get(&resource.rep()).copied().ok_or_else(|| {
            wasmtime::Error::msg(format!("unknown context resource {}", resource.rep()))
        })
    }

    fn proxy_id(&self, resource: &Resource<Proxy>) -> wasmtime::Result<u64> {
        self.proxies.get(&resource.rep()).copied().ok_or_else(|| {
            wasmtime::Error::msg(format!("unknown proxy resource {}", resource.rep()))
        })
    }

    pub(crate) fn proxy_for_rep(&self, rep: u32) -> Option<u64> {
        self.proxies.get(&rep).copied()
    }

    pub(crate) fn ensure_transfer(&self, bytes: usize) -> anyhow::Result<()> {
        if bytes > self.transfer_bytes {
            return Err(anyhow!(TransferLimitError).context(format!(
                "transfer is {bytes} bytes, limit is {} bytes",
                self.transfer_bytes
            )));
        }
        Ok(())
    }

    pub(crate) fn ensure_sample_transfer(&self, sample: &Sample) -> anyhow::Result<()> {
        let bytes = sample
            .tags
            .iter()
            .try_fold(sample.text.len(), |size, tag| {
                size.checked_add(tag.len())
                    .ok_or_else(|| anyhow!(TransferLimitError).context("sample size overflowed"))
            })?;
        self.ensure_transfer(bytes)
    }
}

impl HostContextWithStore<StoreData> for HasSelf<StoreData> {
    fn is_cancelled(
        mut access: Access<StoreData, Self>,
        resource: Resource<Context>,
    ) -> wasmtime::Result<bool> {
        let state = access.get();
        let id = state.context_id(&resource)?;
        state
            .host
            .context_is_cancelled(id)
            .map_err(wasmtime::Error::from_anyhow)
    }

    fn drop(
        mut access: Access<StoreData, Self>,
        resource: Resource<Context>,
    ) -> wasmtime::Result<()> {
        access.get().contexts.remove(&resource.rep());
        Ok(())
    }
}

impl HostProxyWithStore<StoreData> for HasSelf<StoreData> {
    fn transform(
        mut access: Access<StoreData, Self>,
        resource: Resource<Proxy>,
        input: WitSample,
    ) -> wasmtime::Result<Result<WitSample, String>> {
        let state = access.get();
        let id = state.proxy_id(&resource)?;
        let input: Sample = input.into();
        state
            .ensure_sample_transfer(&input)
            .map_err(wasmtime::Error::from_anyhow)?;
        let output = state
            .host
            .proxy_transform(id, input)
            .map_err(wasmtime::Error::from_anyhow)?;
        if let Ok(sample) = &output {
            state
                .ensure_sample_transfer(sample)
                .map_err(wasmtime::Error::from_anyhow)?;
        }
        Ok(output.map(Into::into))
    }

    fn emit_nested(
        mut access: Access<StoreData, Self>,
        resource: Resource<Proxy>,
        input: String,
    ) -> wasmtime::Result<Result<String, String>> {
        let proxy_rep = resource.rep();
        let state = access.get();
        let proxy_id = state.proxy_id(&resource)?;
        state
            .ensure_transfer(input.len())
            .map_err(wasmtime::Error::from_anyhow)?;
        let instance = access
            .get()
            .instance
            .ok_or_else(|| wasmtime::Error::msg("component instance is not active"))?;
        let plugin = bindings::GatePlugin::new(&mut access, &instance)?
            .minekube_gate_spike_plugin()
            .clone();
        let host = Arc::clone(&access.get().host);
        let mut active = ActiveCall::new(access, plugin, proxy_rep);

        let output = host
            .proxy_emit_nested(&mut active, proxy_id, input)
            .map_err(wasmtime::Error::from_anyhow)?;
        active
            .ensure_transfer(
                output
                    .as_ref()
                    .map_or_else(|error| error.len(), String::len),
            )
            .map_err(wasmtime::Error::from_anyhow)?;
        Ok(output)
    }

    fn drop(
        mut access: Access<StoreData, Self>,
        resource: Resource<Proxy>,
    ) -> wasmtime::Result<()> {
        access.get().proxies.remove(&resource.rep());
        Ok(())
    }
}

impl HostContext for StoreData {}
impl HostProxy for StoreData {}
impl bindings::minekube::gate_spike::host::Host for StoreData {}

impl From<WitSample> for Sample {
    fn from(value: WitSample) -> Self {
        Self {
            text: value.text,
            factor: value.factor,
            tags: value.tags,
        }
    }
}

impl From<Sample> for WitSample {
    fn from(value: Sample) -> Self {
        Self {
            text: value.text,
            factor: value.factor,
            tags: value.tags,
        }
    }
}

pub struct Engine {
    store: Store<StoreData>,
    plugin: Guest,
    engine: wasmtime::Engine,
    fuel: u64,
    deadline: Duration,
    memory_bytes: usize,
}

impl Engine {
    pub fn new(component: &[u8], host: Arc<dyn GateHost>, limits: Limits) -> anyhow::Result<Self> {
        let mut config = Config::new();
        config.wasm_component_model(true);
        config.concurrency_support(false);
        config.consume_fuel(true);
        config.epoch_interruption(true);
        let engine = wasmtime::Engine::new(&config)
            .map_err(anyhow::Error::from)
            .context("failed to configure Wasmtime")?;
        let component = Component::new(&engine, component)
            .map_err(anyhow::Error::from)
            .context("failed to compile component")?;
        let mut linker = Linker::new(&engine);
        bindings::GatePlugin::add_to_linker::<_, HasSelf<StoreData>>(&mut linker, |state| state)
            .map_err(anyhow::Error::from)
            .context("failed to link Gate host interface")?;
        let mut store = Store::new(
            &engine,
            StoreData {
                host,
                contexts: HashMap::new(),
                proxies: HashMap::new(),
                next_resource: 1,
                instance: None,
                store_limits: StoreLimitsBuilder::new()
                    .memory_size(limits.memory_bytes)
                    .trap_on_grow_failure(true)
                    .build(),
                transfer_bytes: limits.transfer_bytes,
            },
        );
        store.limiter(|state| &mut state.store_limits);
        store
            .set_fuel(u64::MAX)
            .map_err(anyhow::Error::from)
            .context("failed to configure component instantiation fuel")?;
        store.set_epoch_deadline(u64::MAX);
        let instance = linker
            .instantiate(&mut store, &component)
            .map_err(anyhow::Error::from)
            .context("failed to instantiate component")?;
        let plugin = bindings::GatePlugin::new(&mut store, &instance)
            .map_err(anyhow::Error::from)
            .context("failed to load component exports")?
            .minekube_gate_spike_plugin()
            .clone();
        store.data_mut().instance = Some(instance);

        Ok(Self {
            store,
            plugin,
            engine,
            fuel: limits.fuel,
            deadline: limits.deadline,
            memory_bytes: limits.memory_bytes,
        })
    }

    pub fn init(&mut self, context: u64, proxy: u64) -> anyhow::Result<Sample> {
        let _deadline = self.prepare_call()?;
        let context_resource = self.store.data_mut().insert_context(context)?;
        let context_rep = context_resource.rep();
        let proxy_resource = self.store.data_mut().insert_proxy(proxy)?;
        let proxy_rep = proxy_resource.rep();

        let result = self
            .plugin
            .call_init(&mut self.store, context_resource, proxy_resource)
            .map_err(anyhow::Error::from)
            .context("component init trapped");

        self.store.data_mut().contexts.remove(&context_rep);
        self.store.data_mut().proxies.remove(&proxy_rep);

        let output: Sample = result?
            .map(Into::into)
            .map_err(|message| anyhow!("component init failed: {message}"))?;
        self.store.data().ensure_sample_transfer(&output)?;
        Ok(output)
    }

    pub fn on_event(&mut self, proxy: u64, input: &str) -> anyhow::Result<String> {
        self.store.data().ensure_transfer(input.len())?;
        let _deadline = self.prepare_call()?;
        let proxy_resource = self.store.data_mut().insert_proxy(proxy)?;
        let proxy_rep = proxy_resource.rep();

        let result = self
            .plugin
            .call_on_event(&mut self.store, proxy_resource, input)
            .map_err(anyhow::Error::from)
            .context("component event callback trapped");

        self.store.data_mut().proxies.remove(&proxy_rep);

        let output =
            result?.map_err(|message| anyhow!("component event callback failed: {message}"))?;
        self.store.data().ensure_transfer(output.len())?;
        Ok(output)
    }

    pub fn allocate(&mut self, bytes: u64) -> anyhow::Result<u64> {
        if bytes > u64::try_from(self.memory_bytes).unwrap_or(u64::MAX) {
            return Err(anyhow!(MemoryLimitError).context(format!(
                "requested allocation is {bytes} bytes, limit is {} bytes",
                self.memory_bytes
            )));
        }
        let _deadline = self.prepare_call()?;
        self.plugin
            .call_allocate(&mut self.store, bytes)
            .map_err(anyhow::Error::from)
            .context(MemoryLimitError)
    }

    pub fn spin(&mut self) -> anyhow::Result<()> {
        let _deadline = self.prepare_call()?;
        self.plugin
            .call_spin(&mut self.store)
            .map_err(anyhow::Error::from)
            .context("component spin trapped")
    }

    pub(crate) fn ensure_transfer(&self, bytes: usize) -> anyhow::Result<()> {
        self.store.data().ensure_transfer(bytes)
    }

    fn prepare_call(&mut self) -> anyhow::Result<DeadlineGuard> {
        self.store
            .set_fuel(self.fuel)
            .map_err(anyhow::Error::from)
            .context("failed to reset component fuel")?;
        if self.deadline.is_zero() {
            self.store.set_epoch_deadline(u64::MAX);
            return Ok(DeadlineGuard::disabled());
        }
        self.store.set_epoch_deadline(1);
        DeadlineGuard::new(self.engine.clone(), self.deadline)
    }
}

struct DeadlineGuard {
    cancel: Option<SyncSender<()>>,
    thread: Option<JoinHandle<()>>,
}

impl DeadlineGuard {
    fn disabled() -> Self {
        Self {
            cancel: None,
            thread: None,
        }
    }

    fn new(engine: wasmtime::Engine, deadline: Duration) -> anyhow::Result<Self> {
        let (cancel, receiver) = sync_channel(1);
        let thread = thread::Builder::new()
            .name("gate-wasm-deadline".into())
            .spawn(move || {
                if receiver.recv_timeout(deadline) == Err(mpsc::RecvTimeoutError::Timeout) {
                    engine.increment_epoch();
                }
            })
            .context("failed to start component deadline watchdog")?;
        Ok(Self {
            cancel: Some(cancel),
            thread: Some(thread),
        })
    }
}

impl Drop for DeadlineGuard {
    fn drop(&mut self) {
        if let Some(cancel) = self.cancel.take() {
            let _ = cancel.send(());
        }
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
        }
    }
}

#[cfg(test)]
mod measurements {
    use std::time::Instant;

    use super::*;

    const COMPONENT: &[u8] = include_bytes!("../../artifacts/gate_wasm_spike.component.wasm");

    struct MeasurementHost;

    impl GateHost for MeasurementHost {
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
            active: &mut ActiveCall<'_>,
            proxy: u64,
            input: String,
        ) -> anyhow::Result<Result<String, String>> {
            Ok(Ok(active.on_event(proxy, &input)?))
        }
    }

    #[test]
    #[ignore = "run explicitly to record feasibility measurements"]
    fn measure_cold_compilation_and_instantiation() -> anyhow::Result<()> {
        const COMPILE_ITERATIONS: u32 = 20;
        let compile_started = Instant::now();
        for _ in 0..COMPILE_ITERATIONS {
            let (engine, component, _) = compile_component()?;
            std::hint::black_box((engine, component));
        }
        let compile_ns = compile_started.elapsed().as_nanos() / u128::from(COMPILE_ITERATIONS);

        let (engine, component, linker) = compile_component()?;
        const INSTANTIATE_ITERATIONS: u32 = 100;
        let instantiate_started = Instant::now();
        for _ in 0..INSTANTIATE_ITERATIONS {
            let instance = instantiate_component(&engine, &component, &linker)?;
            std::hint::black_box(instance);
        }
        let instantiate_ns =
            instantiate_started.elapsed().as_nanos() / u128::from(INSTANTIATE_ITERATIONS);

        eprintln!("measurement:cold-compilation-ns={compile_ns}");
        eprintln!("measurement:instantiation-ns={instantiate_ns}");
        Ok(())
    }

    fn compile_component() -> anyhow::Result<(wasmtime::Engine, Component, Linker<StoreData>)> {
        let mut config = Config::new();
        config.wasm_component_model(true);
        config.concurrency_support(false);
        config.consume_fuel(true);
        config.epoch_interruption(true);
        let engine = wasmtime::Engine::new(&config)?;
        let component = Component::new(&engine, COMPONENT)?;
        let mut linker = Linker::new(&engine);
        bindings::GatePlugin::add_to_linker::<_, HasSelf<StoreData>>(&mut linker, |state| state)?;
        Ok((engine, component, linker))
    }

    fn instantiate_component(
        engine: &wasmtime::Engine,
        component: &Component,
        linker: &Linker<StoreData>,
    ) -> anyhow::Result<Store<StoreData>> {
        let limits = Limits::default();
        let mut store = Store::new(
            engine,
            StoreData {
                host: Arc::new(MeasurementHost),
                contexts: HashMap::new(),
                proxies: HashMap::new(),
                next_resource: 1,
                instance: None,
                store_limits: StoreLimitsBuilder::new()
                    .memory_size(limits.memory_bytes)
                    .trap_on_grow_failure(true)
                    .build(),
                transfer_bytes: limits.transfer_bytes,
            },
        );
        store.limiter(|state| &mut state.store_limits);
        store.set_fuel(u64::MAX)?;
        store.set_epoch_deadline(u64::MAX);
        let instance = linker.instantiate(&mut store, component)?;
        let _plugin = bindings::GatePlugin::new(&mut store, &instance)?
            .minekube_gate_spike_plugin()
            .clone();
        store.data_mut().instance = Some(instance);
        Ok(store)
    }
}
