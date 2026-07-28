use std::collections::HashMap;
use std::sync::Arc;

use anyhow::{Context as _, anyhow};
use wasmtime::component::{Component, HasSelf, Instance, Linker, Resource};
use wasmtime::{Config, Store};

use crate::{Host as GateHost, Limits, Sample};

pub(crate) mod bindings {
    wasmtime::component::bindgen!({
        path: "../wit",
        world: "gate-plugin",
        imports: { default: trappable },
    });
}

use bindings::exports::minekube::gate_spike::plugin::Guest;
use bindings::minekube::gate_spike::host::{
    Context, HostContext, HostProxy, Proxy, Sample as WitSample,
};

struct StoreData {
    host: Arc<dyn GateHost>,
    contexts: HashMap<u32, u64>,
    proxies: HashMap<u32, u64>,
    next_resource: u32,
    instance: Option<Instance>,
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
}

impl HostContext for StoreData {
    fn is_cancelled(&mut self, resource: Resource<Context>) -> wasmtime::Result<bool> {
        let id = self.context_id(&resource)?;
        self.host
            .context_is_cancelled(id)
            .map_err(wasmtime::Error::from_anyhow)
    }

    fn drop(&mut self, resource: Resource<Context>) -> wasmtime::Result<()> {
        self.contexts.remove(&resource.rep());
        Ok(())
    }
}

impl HostProxy for StoreData {
    fn transform(
        &mut self,
        resource: Resource<Proxy>,
        input: WitSample,
    ) -> wasmtime::Result<Result<WitSample, String>> {
        let id = self.proxy_id(&resource)?;
        let output = self
            .host
            .proxy_transform(id, input.into())
            .map_err(wasmtime::Error::from_anyhow)?;
        Ok(output.map(Into::into))
    }

    fn emit_nested(
        &mut self,
        _resource: Resource<Proxy>,
        _input: String,
    ) -> wasmtime::Result<Result<String, String>> {
        Err(wasmtime::Error::msg(
            "nested component re-entry is not active",
        ))
    }

    fn drop(&mut self, resource: Resource<Proxy>) -> wasmtime::Result<()> {
        self.proxies.remove(&resource.rep());
        Ok(())
    }
}

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
}

impl Engine {
    pub fn new(component: &[u8], host: Arc<dyn GateHost>, limits: Limits) -> anyhow::Result<Self> {
        let mut config = Config::new();
        config.wasm_component_model(true);
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
            },
        );
        store
            .set_fuel(limits.fuel)
            .map_err(anyhow::Error::from)
            .context("failed to configure component fuel")?;
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

        Ok(Self { store, plugin })
    }

    pub fn init(&mut self, context: u64, proxy: u64) -> anyhow::Result<Sample> {
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

        result?
            .map(Into::into)
            .map_err(|message| anyhow!("component init failed: {message}"))
    }
}
