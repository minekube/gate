//go:build wasm_native && cgo

#include "bridge.h"

#if defined(_WIN32)
#include <windows.h>
#else
#include <pthread.h>
#endif

int32_t gate_wasm_reentry_call(
    gate_wasm_reentry *reentry, uint64_t proxy_id, gate_wasm_slice input,
    gate_wasm_owned_bytes *output, gate_wasm_owned_bytes *error) {
  return gate_wasm_reentry_on_event(reentry, proxy_id, input, output, error);
}

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

void gate_wasm_owned_bytes_list_set(
    gate_wasm_owned_bytes_list *value, gate_wasm_owned_bytes *ptr, size_t len,
    size_t cap) {
  value->ptr = ptr;
  value->len = len;
  value->cap = cap;
}
