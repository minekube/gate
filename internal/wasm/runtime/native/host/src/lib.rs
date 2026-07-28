mod engine;
pub mod reentry;

pub use engine::Engine;
pub use reentry::ActiveCall;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Sample {
    pub text: String,
    pub factor: i32,
    pub tags: Vec<String>,
}

#[derive(Clone, Copy, Debug)]
pub struct Limits {
    pub memory_bytes: usize,
    pub transfer_bytes: usize,
    pub fuel: u64,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            memory_bytes: 128 * 1024 * 1024,
            transfer_bytes: 16 * 1024 * 1024,
            fuel: 10_000_000,
        }
    }
}

pub trait Host: Send + Sync + 'static {
    fn context_is_cancelled(&self, context: u64) -> anyhow::Result<bool>;

    fn proxy_transform(&self, proxy: u64, input: Sample) -> anyhow::Result<Result<Sample, String>>;

    fn proxy_emit_nested(
        &self,
        active: &mut ActiveCall<'_>,
        proxy: u64,
        input: String,
    ) -> anyhow::Result<Result<String, String>>;
}
