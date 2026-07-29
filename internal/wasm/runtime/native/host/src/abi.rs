use std::ptr;

use anyhow::anyhow;

#[repr(C)]
#[derive(Clone, Copy)]
pub struct Slice {
    pub ptr: *const u8,
    pub len: usize,
}

impl Slice {
    pub unsafe fn copy(self) -> anyhow::Result<Vec<u8>> {
        if self.len == 0 {
            return Ok(Vec::new());
        }
        if self.ptr.is_null() {
            return Err(anyhow!("non-empty input has a null pointer"));
        }
        // SAFETY: The C ABI requires the caller to keep this allocation readable
        // for the duration of the call. We immediately copy it.
        Ok(unsafe { std::slice::from_raw_parts(self.ptr, self.len) }.to_vec())
    }
}

#[repr(C)]
pub struct OwnedBytes {
    pub ptr: *mut u8,
    pub len: usize,
    pub cap: usize,
}

impl Default for OwnedBytes {
    fn default() -> Self {
        Self {
            ptr: ptr::null_mut(),
            len: 0,
            cap: 0,
        }
    }
}

impl OwnedBytes {
    pub fn from_vec(mut bytes: Vec<u8>) -> Self {
        if bytes.is_empty() {
            return Self::default();
        }
        let value = Self {
            ptr: bytes.as_mut_ptr(),
            len: bytes.len(),
            cap: bytes.capacity(),
        };
        std::mem::forget(bytes);
        value
    }

    pub fn from_string(value: String) -> Self {
        Self::from_vec(value.into_bytes())
    }

    pub unsafe fn free_rust(self) {
        if self.cap == 0 {
            return;
        }
        // SAFETY: Rust-created outputs preserve the exact Vec pointer, length,
        // and capacity in this structure and are freed exactly once.
        drop(unsafe { Vec::from_raw_parts(self.ptr, self.len, self.cap) });
    }

    pub unsafe fn copy_and_free_c(self) -> anyhow::Result<Vec<u8>> {
        let bytes = if self.len == 0 {
            Vec::new()
        } else {
            if self.ptr.is_null() {
                return Err(anyhow!("non-empty callback output has a null pointer"));
            }
            // SAFETY: Go allocated this buffer with C.malloc and transfers
            // ownership to this callback invocation.
            unsafe { std::slice::from_raw_parts(self.ptr, self.len) }.to_vec()
        };
        if !self.ptr.is_null() {
            // SAFETY: Callback buffers are allocated with C.malloc.
            unsafe { libc::free(self.ptr.cast()) };
        }
        Ok(bytes)
    }
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct AbiLimits {
    pub memory_bytes: u64,
    pub transfer_bytes: u64,
    pub fuel: u64,
    pub deadline_nanos: u64,
}
