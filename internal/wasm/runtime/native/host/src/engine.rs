use std::collections::HashMap;
use std::marker::PhantomData;
use std::rc::Rc;
use std::sync::mpsc::{SyncSender, sync_channel};
use std::sync::{Arc, mpsc};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use anyhow::{Context as _, anyhow, bail};
use wasmtime::component::{
    Access, Component, Func, HasSelf, Instance, Linker, Resource, ResourceDynamic, Val,
    types::ComponentFunc,
};
use wasmtime::{AsContextMut, Config, Store, StoreContextMut, StoreLimits, StoreLimitsBuilder};

use crate::{ActiveCall, Host as GateHost, Limits, Sample, TransferLimitError};
use crate::{generated, wire};

pub(crate) mod bindings {
    wasmtime::component::bindgen!({
        path: "../wit",
        world: "gate-plugin",
        imports: { default: trappable | store },
    });
}

use bindings::minekube::gate_spike::host::{
    Context, HostContext, HostContextWithStore, HostProxy, HostProxyWithStore, Proxy,
    Sample as WitSample,
};

pub(crate) struct StoreData {
    host: Arc<dyn GateHost>,
    contexts: HashMap<u32, u64>,
    proxies: HashMap<u32, u64>,
    resources: HashMap<u32, HostResource>,
    callbacks: HashMap<u32, Func>,
    next_resource: u32,
    instance: Option<Instance>,
    store_limits: StoreLimits,
    transfer_bytes: usize,
}

#[derive(Clone, Copy)]
struct HostResource {
    handle: u64,
    type_id: u32,
    owned: bool,
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

    fn insert_gate_resource(
        &mut self,
        handle: u64,
        resource_name: &str,
        owned: bool,
    ) -> anyhow::Result<(u32, u32)> {
        let descriptor = generated::dispatch::RESOURCE_TYPES
            .iter()
            .find(|resource| resource.name == resource_name)
            .with_context(|| format!("generated resource {resource_name} is missing"))?;
        let representation =
            <Self as wire::ResourceTransport>::insert_resource(self, handle, descriptor.id, owned)?;
        Ok((representation, descriptor.id))
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

impl wire::ResourceTransport for StoreData {
    fn resource_handle(&self, representation: u32) -> anyhow::Result<u64> {
        self.resources
            .get(&representation)
            .map(|resource| resource.handle)
            .with_context(|| format!("unknown Gate resource representation {representation}"))
    }

    fn insert_resource(&mut self, handle: u64, type_id: u32, owned: bool) -> anyhow::Result<u32> {
        let representation = self.next_rep()?;
        self.resources.insert(
            representation,
            HostResource {
                handle,
                type_id,
                owned,
            },
        );
        Ok(representation)
    }
}

impl generated::dispatch::Dispatch for StoreData {
    fn invoke(
        mut store: StoreContextMut<'_, Self>,
        operation: &'static generated::dispatch::Operation,
        function_type: ComponentFunc,
        parameters: &[Val],
        results: &mut [Val],
    ) -> wasmtime::Result<()> {
        let request = wire::encode_parameters(store.as_context_mut(), parameters)
            .map_err(wasmtime::Error::from_anyhow)?;
        store
            .data()
            .ensure_transfer(request.len())
            .map_err(wasmtime::Error::from_anyhow)?;
        let host = Arc::clone(&store.data().host);
        let mut active = CallbackCall::new(store);
        let response = host
            .invoke(&mut active, operation.id, &request)
            .map_err(wasmtime::Error::from_anyhow)?;
        active
            .store
            .data()
            .ensure_transfer(response.len())
            .map_err(wasmtime::Error::from_anyhow)?;
        wire::decode_results(
            active.store.as_context_mut(),
            &function_type,
            &response,
            results,
        )
        .map_err(wasmtime::Error::from_anyhow)
    }

    fn register_callback(
        mut store: StoreContextMut<'_, Self>,
        callback: &'static generated::dispatch::CallbackDescriptor,
        guest_id: u64,
        results: &mut [Val],
    ) -> wasmtime::Result<()> {
        let handle = store
            .data()
            .host
            .register_callback(callback.id, guest_id)
            .map_err(wasmtime::Error::from_anyhow)?;
        let representation = <Self as wire::ResourceTransport>::insert_resource(
            store.data_mut(),
            handle,
            callback.type_id,
            true,
        )
        .map_err(wasmtime::Error::from_anyhow)?;
        let resource = ResourceDynamic::new_own(representation, callback.type_id)
            .try_into_resource_any(&mut store)?;
        let [result] = results else {
            return Err(wasmtime::Error::msg(
                "callback constructor expected one resource result",
            ));
        };
        *result = Val::Resource(resource);
        Ok(())
    }

    fn call_callback(
        mut store: StoreContextMut<'_, Self>,
        callback: &'static generated::dispatch::CallbackDescriptor,
        function_type: ComponentFunc,
        parameters: &[Val],
        results: &mut [Val],
    ) -> wasmtime::Result<()> {
        let request = wire::encode_parameters(store.as_context_mut(), parameters)
            .map_err(wasmtime::Error::from_anyhow)?;
        store
            .data()
            .ensure_transfer(request.len())
            .map_err(wasmtime::Error::from_anyhow)?;
        let host = Arc::clone(&store.data().host);
        let mut active = CallbackCall::new(store);
        let response = host
            .invoke(&mut active, (1_u32 << 31) | callback.id, &request)
            .map_err(wasmtime::Error::from_anyhow)?;
        active
            .store
            .data()
            .ensure_transfer(response.len())
            .map_err(wasmtime::Error::from_anyhow)?;
        wire::decode_results(
            active.store.as_context_mut(),
            &function_type,
            &response,
            results,
        )
        .map_err(wasmtime::Error::from_anyhow)
    }

    fn drop_resource(
        mut store: StoreContextMut<'_, Self>,
        resource: &'static generated::dispatch::ResourceDescriptor,
        representation: u32,
    ) -> wasmtime::Result<()> {
        let entry = store
            .data_mut()
            .resources
            .remove(&representation)
            .with_context(|| format!("unknown resource representation {representation}"))
            .map_err(wasmtime::Error::from_anyhow)?;
        if entry.type_id != resource.id {
            return Err(wasmtime::Error::msg(format!(
                "resource representation {representation} has type {}, expected {}",
                entry.type_id, resource.id
            )));
        }
        if entry.owned {
            store
                .data()
                .host
                .drop_resource(entry.handle)
                .map_err(wasmtime::Error::from_anyhow)?;
        }
        Ok(())
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

/// A synchronous, thread-bound callback entry into the currently active
/// generated Gate API call.
pub struct CallbackCall<'a> {
    store: StoreContextMut<'a, StoreData>,
    _thread_bound: PhantomData<Rc<()>>,
}

impl<'a> CallbackCall<'a> {
    fn new(store: StoreContextMut<'a, StoreData>) -> Self {
        Self {
            store,
            _thread_bound: PhantomData,
        }
    }

    pub fn invoke_callback(
        &mut self,
        callback_type: u32,
        guest_id: u64,
        input: &[u8],
    ) -> anyhow::Result<Vec<u8>> {
        self.store.data().ensure_transfer(input.len())?;
        let callback = self
            .store
            .data()
            .callbacks
            .get(&callback_type)
            .cloned()
            .with_context(|| format!("unknown generated callback type {callback_type}"))?;
        let function_type = callback.ty(&self.store);
        let parameters = wire::decode_callback_parameters(
            self.store.as_context_mut(),
            &function_type,
            guest_id,
            input,
        )?;
        let mut results = vec![Val::Bool(false); function_type.results().len()];
        callback
            .call(&mut self.store, &parameters, &mut results)
            .map_err(anyhow::Error::from)
            .with_context(|| format!("component callback {callback_type} trapped"))?;
        let output =
            wire::encode_callback_results(self.store.as_context_mut(), &function_type, &results)?;
        self.store.data().ensure_transfer(output.len())?;
        Ok(output)
    }
}

pub struct Engine {
    store: Store<StoreData>,
    metadata: PluginMetadata,
    init: Func,
    engine: wasmtime::Engine,
    fuel: u64,
    deadline: Duration,
    memory_bytes: usize,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct PluginMetadata {
    pub name: String,
    pub version: String,
    pub contract_hash: String,
    pub generator_format: u32,
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
        generated::dispatch::add_to_linker(&mut linker)
            .map_err(anyhow::Error::from)
            .context("failed to link generated Gate host interfaces")?;
        let mut store = Store::new(
            &engine,
            StoreData {
                host,
                contexts: HashMap::new(),
                proxies: HashMap::new(),
                resources: HashMap::new(),
                callbacks: HashMap::new(),
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
        let plugin = instance
            .get_export_index(&mut store, None, "minekube:gate/plugin@0.1.0")
            .context("component does not export minekube:gate/plugin@0.1.0")?;
        let metadata_export = instance
            .get_export_index(&mut store, Some(&plugin), "metadata")
            .and_then(|index| instance.get_func(&mut store, &index))
            .context("component does not export plugin metadata")?;
        let init = instance
            .get_export_index(&mut store, Some(&plugin), "init")
            .and_then(|index| instance.get_func(&mut store, &index))
            .context("component does not export plugin init")?;
        let mut callbacks = HashMap::new();
        for callback in generated::dispatch::CALLBACKS {
            let name = format!("invoke-{}", callback.name);
            let function = instance
                .get_export_index(&mut store, Some(&plugin), &name)
                .and_then(|index| instance.get_func(&mut store, &index))
                .with_context(|| format!("component does not export plugin {name}"))?;
            callbacks.insert(callback.id, function);
        }
        store.data_mut().callbacks = callbacks;
        store.data_mut().instance = Some(instance);

        let mut engine = Self {
            store,
            metadata: PluginMetadata {
                name: String::new(),
                version: String::new(),
                contract_hash: String::new(),
                generator_format: 0,
            },
            init,
            engine,
            fuel: limits.fuel,
            deadline: limits.deadline,
            memory_bytes: limits.memory_bytes,
        };
        engine.metadata = engine.read_metadata(metadata_export)?;
        Ok(engine)
    }

    pub fn init(&mut self, context: u64, proxy: u64) -> anyhow::Result<Sample> {
        let _deadline = self.prepare_call()?;
        let (context_rep, context_type) =
            self.store
                .data_mut()
                .insert_gate_resource(context, "context-e30d9213847b", false)?;
        let (proxy_rep, proxy_type) =
            self.store
                .data_mut()
                .insert_gate_resource(proxy, "proxy-3cf24d6ad4bb", false)?;
        // Dynamic ResourceAny values must be rooted as owned outside an
        // active component call. The exported init signature borrows them, and
        // we explicitly drop the temporary roots immediately afterwards.
        let context_resource = ResourceDynamic::new_own(context_rep, context_type)
            .try_into_resource_any(&mut self.store)?;
        let proxy_resource = ResourceDynamic::new_own(proxy_rep, proxy_type)
            .try_into_resource_any(&mut self.store)?;
        let mut results = vec![Val::Bool(false); self.init.ty(&self.store).results().len()];
        let mut parameters = vec![
            Val::Resource(context_resource),
            Val::Resource(proxy_resource),
        ];
        let result = self
            .init
            .call(&mut self.store, &parameters, &mut results)
            .map_err(anyhow::Error::from)
            .context("component init trapped");
        for parameter in parameters.drain(..) {
            if let Val::Resource(resource) = parameter {
                resource.resource_drop(&mut self.store)?;
            }
        }
        self.store.data_mut().resources.remove(&context_rep);
        self.store.data_mut().resources.remove(&proxy_rep);
        result?;
        match results.as_slice() {
            [Val::Result(Ok(None))] => Ok(Sample {
                text: String::new(),
                factor: 0,
                tags: Vec::new(),
            }),
            [Val::Result(Err(Some(error)))] => {
                Err(anyhow!("component init failed: {}", gate_error(error)))
            }
            other => Err(anyhow!(
                "component init returned an invalid value: {other:?}"
            )),
        }
    }

    fn read_metadata(&mut self, metadata: Func) -> anyhow::Result<PluginMetadata> {
        let mut results = vec![Val::Bool(false); metadata.ty(&self.store).results().len()];
        metadata
            .call(&mut self.store, &[], &mut results)
            .map_err(anyhow::Error::from)
            .context("component metadata trapped")?;
        let [Val::Record(fields)] = results.as_slice() else {
            bail!("component metadata returned an invalid value");
        };
        let name = record_string(fields, "name")?;
        if name.trim().is_empty() {
            bail!("component metadata name is empty");
        }
        let version = record_string(fields, "version")?;
        let contract_hash = record_string(fields, "contract-hash")?;
        if contract_hash != generated::dispatch::WIT_HASH {
            bail!(
                "component contract hash {contract_hash} does not match Gate {}",
                generated::dispatch::WIT_HASH
            );
        }
        let generator_format = record_u32(fields, "generator-format")?;
        if generator_format != generated::bindings::GENERATOR_FORMAT {
            bail!(
                "component generator format {generator_format} does not match Gate {}",
                generated::bindings::GENERATOR_FORMAT
            );
        }
        Ok(PluginMetadata {
            name,
            version,
            contract_hash,
            generator_format,
        })
    }

    pub fn metadata(&self) -> &PluginMetadata {
        &self.metadata
    }

    pub fn invoke_callback(
        &mut self,
        callback_type: u32,
        guest_id: u64,
        input: &[u8],
    ) -> anyhow::Result<Vec<u8>> {
        let callback = self
            .store
            .data()
            .callbacks
            .get(&callback_type)
            .cloned()
            .with_context(|| format!("unknown generated callback type {callback_type}"))?;
        let _deadline = self.prepare_call()?;
        self.store.data().ensure_transfer(input.len())?;
        let function_type = callback.ty(&self.store);
        let parameters = wire::decode_callback_parameters(
            self.store.as_context_mut(),
            &function_type,
            guest_id,
            input,
        )?;
        let mut results = vec![Val::Bool(false); function_type.results().len()];
        callback
            .call(&mut self.store, &parameters, &mut results)
            .map_err(anyhow::Error::from)
            .with_context(|| format!("component callback {callback_type} trapped"))?;
        let output =
            wire::encode_callback_results(self.store.as_context_mut(), &function_type, &results)?;
        self.store.data().ensure_transfer(output.len())?;
        Ok(output)
    }

    pub fn on_event(&mut self, proxy: u64, input: &str) -> anyhow::Result<String> {
        let _ = (proxy, input);
        bail!("direct spike event entrypoint is not part of the generated Gate component contract")
    }

    pub fn allocate(&mut self, bytes: u64) -> anyhow::Result<u64> {
        let _ = bytes;
        bail!("spike allocation entrypoint is unavailable")
    }

    pub fn spin(&mut self) -> anyhow::Result<()> {
        bail!("spike spin entrypoint is unavailable")
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

fn record_string(fields: &[(String, Val)], name: &str) -> anyhow::Result<String> {
    fields
        .iter()
        .find(|(field, _)| field == name)
        .and_then(|(_, value)| match value {
            Val::String(value) => Some(value.clone()),
            _ => None,
        })
        .with_context(|| format!("component metadata field {name} is missing"))
}

fn record_u32(fields: &[(String, Val)], name: &str) -> anyhow::Result<u32> {
    fields
        .iter()
        .find(|(field, _)| field == name)
        .and_then(|(_, value)| match value {
            Val::U32(value) => Some(*value),
            _ => None,
        })
        .with_context(|| format!("component metadata field {name} is missing"))
}

fn gate_error(value: &Val) -> String {
    let Val::Record(fields) = value else {
        return format!("{value:?}");
    };
    let kind = record_string(fields, "kind").unwrap_or_else(|_| "gate-error".into());
    let message = record_string(fields, "message").unwrap_or_else(|_| "unknown error".into());
    let operation = record_string(fields, "operation").unwrap_or_default();
    if operation.is_empty() {
        format!("{kind}: {message}")
    } else {
        format!("{operation}: {kind}: {message}")
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

#[cfg(any())]
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
