use std::marker::PhantomData;
use std::rc::Rc;

use anyhow::anyhow;
use wasmtime::component::{Access, HasSelf, Resource};

use crate::engine::{StoreData, bindings};

/// A synchronous, thread-bound view of the currently executing component call.
///
/// The token cannot be retained beyond the host import that receives it:
///
/// ```compile_fail
/// use gate_wasm_native::ActiveCall;
///
/// fn retain_after_import<'call>(
///     active: &'call mut ActiveCall<'call>,
/// ) -> &'static mut ActiveCall<'static> {
///     active
/// }
/// ```
pub struct ActiveCall<'a> {
    access: Access<'a, StoreData, HasSelf<StoreData>>,
    plugin: bindings::exports::minekube::gate_spike::plugin::Guest,
    proxy_rep: u32,
    _thread_bound: PhantomData<Rc<()>>,
}

impl<'a> ActiveCall<'a> {
    pub(crate) fn new(
        access: Access<'a, StoreData, HasSelf<StoreData>>,
        plugin: bindings::exports::minekube::gate_spike::plugin::Guest,
        proxy_rep: u32,
    ) -> Self {
        Self {
            access,
            plugin,
            proxy_rep,
            _thread_bound: PhantomData,
        }
    }

    pub fn on_event(&mut self, proxy: u64, input: &str) -> anyhow::Result<String> {
        let active_proxy = self
            .access
            .data_mut()
            .proxy_for_rep(self.proxy_rep)
            .ok_or_else(|| anyhow!("active proxy resource expired"))?;
        if active_proxy != proxy {
            return Err(anyhow!(
                "active proxy resource is {active_proxy}, requested {proxy}"
            ));
        }

        let resource =
            Resource::<bindings::minekube::gate_spike::host::Proxy>::new_borrow(self.proxy_rep);
        self.plugin
            .call_on_event(&mut self.access, resource, input)
            .map_err(anyhow::Error::from)?
            .map_err(|message| anyhow!("nested component event failed: {message}"))
    }
}
