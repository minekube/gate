use std::ptr;

use anyhow::{Context as _, anyhow};

use crate::Sample;

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
pub struct SliceList {
    pub ptr: *const Slice,
    pub len: usize,
}

#[repr(C)]
pub struct OwnedBytesList {
    pub ptr: *mut OwnedBytes,
    pub len: usize,
    pub cap: usize,
}

impl Default for OwnedBytesList {
    fn default() -> Self {
        Self {
            ptr: ptr::null_mut(),
            len: 0,
            cap: 0,
        }
    }
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct SampleView {
    pub text: Slice,
    pub factor: i32,
    pub tags: SliceList,
}

#[repr(C)]
#[derive(Default)]
pub struct OwnedSample {
    pub text: OwnedBytes,
    pub factor: i32,
    pub tags: OwnedBytesList,
}

impl OwnedSample {
    pub fn from_sample(sample: Sample) -> Self {
        let mut tags: Vec<_> = sample
            .tags
            .into_iter()
            .map(OwnedBytes::from_string)
            .collect();
        let tag_list = if tags.is_empty() {
            OwnedBytesList::default()
        } else {
            let list = OwnedBytesList {
                ptr: tags.as_mut_ptr(),
                len: tags.len(),
                cap: tags.capacity(),
            };
            std::mem::forget(tags);
            list
        };
        Self {
            text: OwnedBytes::from_string(sample.text),
            factor: sample.factor,
            tags: tag_list,
        }
    }

    pub unsafe fn free_rust(self) {
        // SAFETY: All fields were created by OwnedSample::from_sample.
        unsafe { self.text.free_rust() };
        if self.tags.cap == 0 {
            return;
        }
        // SAFETY: The list preserves the original Vec allocation.
        let tags = unsafe { Vec::from_raw_parts(self.tags.ptr, self.tags.len, self.tags.cap) };
        for tag in tags {
            // SAFETY: Each item was created by OwnedBytes::from_string.
            unsafe { tag.free_rust() };
        }
    }

    pub unsafe fn copy_and_free_c(self) -> anyhow::Result<Sample> {
        // Copy every field before freeing the C allocations, and attempt to
        // release all allocations even if UTF-8 validation fails.
        // SAFETY: Go callback outputs use C.malloc ownership.
        let text = unsafe { self.text.copy_and_free_c() };
        let tags = if self.tags.len == 0 {
            Vec::new()
        } else {
            if self.tags.ptr.is_null() {
                return Err(anyhow!("non-empty callback tag list has a null pointer"));
            }
            // SAFETY: Go allocated a contiguous OwnedBytes array.
            let items = unsafe { std::slice::from_raw_parts(self.tags.ptr, self.tags.len) };
            let mut result = Vec::with_capacity(items.len());
            for item in items {
                // Copy the value because ownership is encoded in its fields.
                let owned = OwnedBytes {
                    ptr: item.ptr,
                    len: item.len,
                    cap: item.cap,
                };
                // SAFETY: Each callback tag is independently C-allocated.
                result.push(unsafe { owned.copy_and_free_c() });
            }
            result
        };
        if !self.tags.ptr.is_null() {
            // SAFETY: Go allocated the tag array with C.malloc.
            unsafe { libc::free(self.tags.ptr.cast()) };
        }

        let text = String::from_utf8(text?).context("callback text is not UTF-8")?;
        let tags = tags
            .into_iter()
            .map(|tag| String::from_utf8(tag?).context("callback tag is not UTF-8"))
            .collect::<anyhow::Result<Vec<_>>>()?;
        Ok(Sample {
            text,
            factor: self.factor,
            tags,
        })
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
