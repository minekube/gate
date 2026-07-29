use std::collections::HashMap;
use std::marker::PhantomData;
use std::rc::Rc;
use std::sync::mpsc::{SyncSender, sync_channel};
use std::sync::{Arc, mpsc};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use anyhow::{Context as _, anyhow, bail};
use wasmtime::component::{Component, Func, Linker, ResourceDynamic, Val, types::ComponentFunc};
use wasmtime::{AsContextMut, Config, Store, StoreContextMut, StoreLimits, StoreLimitsBuilder};

use crate::{Host as GateHost, Limits, TransferLimitError};
use crate::{generated, wire};

pub(crate) struct StoreData {
    host: Arc<dyn GateHost>,
    resources: HashMap<u32, HostResource>,
    callbacks: HashMap<u32, Func>,
    next_resource: u32,
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
    fn next_rep(&mut self) -> anyhow::Result<u32> {
        let rep = self.next_resource;
        self.next_resource = self
            .next_resource
            .checked_add(1)
            .ok_or_else(|| anyhow!("resource representation exhausted"))?;
        Ok(rep)
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

    fn resource_scope(&self) -> u32 {
        self.next_resource
    }

    fn release_borrowed_since(&mut self, first: u32) {
        release_borrowed_resources(&mut self.resources, first);
    }
}

fn release_borrowed_resources(resources: &mut HashMap<u32, HostResource>, first: u32) {
    resources.retain(|representation, resource| *representation < first || resource.owned);
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
        let scope = self.store.data().resource_scope();
        let result = (|| {
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
            let output = wire::encode_callback_results(
                self.store.as_context_mut(),
                &function_type,
                &results,
            )?;
            self.store.data().ensure_transfer(output.len())?;
            Ok(output)
        })();
        self.store.data_mut().release_borrowed_since(scope);
        result
    }
}

pub struct Engine {
    store: Store<StoreData>,
    metadata: PluginMetadata,
    init: Func,
    engine: wasmtime::Engine,
    fuel: u64,
    deadline: Duration,
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
                resources: HashMap::new(),
                callbacks: HashMap::new(),
                next_resource: 1,
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
        };
        engine.metadata = engine.read_metadata(metadata_export)?;
        Ok(engine)
    }

    pub fn init(&mut self, context: u64, proxy: u64) -> anyhow::Result<()> {
        let _deadline = self.prepare_call()?;
        let scope = self.store.data().resource_scope();
        let result = self.init_scoped(context, proxy);
        self.store.data_mut().release_borrowed_since(scope);
        result
    }

    fn init_scoped(&mut self, context: u64, proxy: u64) -> anyhow::Result<()> {
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
        let mut drop_error = None;
        for parameter in parameters.drain(..) {
            if let Val::Resource(resource) = parameter {
                if let Err(error) = resource.resource_drop(&mut self.store) {
                    drop_error.get_or_insert(error);
                }
            }
        }
        self.store.data_mut().resources.remove(&context_rep);
        self.store.data_mut().resources.remove(&proxy_rep);
        result?;
        if let Some(error) = drop_error {
            return Err(error.into());
        }
        match results.as_slice() {
            [Val::Result(Ok(None))] => Ok(()),
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
        let _deadline = self.prepare_call()?;
        let scope = self.store.data().resource_scope();
        let result = (|| {
            let callback = self
                .store
                .data()
                .callbacks
                .get(&callback_type)
                .cloned()
                .with_context(|| format!("unknown generated callback type {callback_type}"))?;
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
            let output = wire::encode_callback_results(
                self.store.as_context_mut(),
                &function_type,
                &results,
            )?;
            self.store.data().ensure_transfer(output.len())?;
            Ok(output)
        })();
        self.store.data_mut().release_borrowed_since(scope);
        result
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

#[cfg(test)]
mod tests {
    use super::{HostResource, release_borrowed_resources};
    use std::collections::HashMap;

    #[test]
    fn callback_scope_releases_only_new_borrowed_resources() {
        let mut resources = HashMap::from([
            (
                3,
                HostResource {
                    handle: 30,
                    type_id: 1,
                    owned: false,
                },
            ),
            (
                4,
                HostResource {
                    handle: 40,
                    type_id: 1,
                    owned: false,
                },
            ),
            (
                5,
                HostResource {
                    handle: 50,
                    type_id: 1,
                    owned: true,
                },
            ),
        ]);

        release_borrowed_resources(&mut resources, 4);

        assert!(resources.contains_key(&3));
        assert!(!resources.contains_key(&4));
        assert!(resources.contains_key(&5));
    }
}
