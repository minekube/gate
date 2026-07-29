//go:build wasm_native && cgo

#include "bridge.h"

#if defined(_WIN32)
#include <windows.h>
#else
#include <pthread.h>
#endif

uintptr_t gate_wasm_current_thread_id(void) {
#if defined(_WIN32)
  return (uintptr_t)GetCurrentThreadId();
#elif defined(__APPLE__)
  uint64_t thread_id = 0;
  if (pthread_threadid_np(NULL, &thread_id) != 0) {
    return 0;
  }
  return (uintptr_t)thread_id;
#else
  return (uintptr_t)pthread_self();
#endif
}

void gate_wasm_owned_bytes_set(
    gate_wasm_owned_bytes *value, uint8_t *ptr, size_t len, size_t cap) {
  value->ptr = ptr;
  value->len = len;
  value->cap = cap;
}
