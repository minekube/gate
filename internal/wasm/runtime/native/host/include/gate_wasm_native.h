#ifndef GATE_WASM_NATIVE_H
#define GATE_WASM_NATIVE_H

#include <stddef.h>
#include <stdint.h>

typedef struct gate_wasm_slice {
  const uint8_t *ptr;
  size_t len;
} gate_wasm_slice;

typedef struct gate_wasm_owned_bytes {
  uint8_t *ptr;
  size_t len;
  size_t cap;
} gate_wasm_owned_bytes;

typedef struct gate_wasm_slice_list {
  const gate_wasm_slice *ptr;
  size_t len;
} gate_wasm_slice_list;

typedef struct gate_wasm_owned_bytes_list {
  gate_wasm_owned_bytes *ptr;
  size_t len;
  size_t cap;
} gate_wasm_owned_bytes_list;

typedef struct gate_wasm_sample_view {
  gate_wasm_slice text;
  int32_t factor;
  gate_wasm_slice_list tags;
} gate_wasm_sample_view;

typedef struct gate_wasm_owned_sample {
  gate_wasm_owned_bytes text;
  int32_t factor;
  gate_wasm_owned_bytes_list tags;
} gate_wasm_owned_sample;

typedef struct gate_wasm_limits {
  uint64_t memory_bytes;
  uint64_t transfer_bytes;
  uint64_t fuel;
  uint64_t deadline_nanos;
} gate_wasm_limits;

typedef struct gate_wasm_plugin_metadata {
  gate_wasm_slice name;
  gate_wasm_slice version;
  gate_wasm_slice contract_hash;
  uint32_t generator_format;
} gate_wasm_plugin_metadata;

typedef struct gate_wasm_runtime gate_wasm_runtime;
typedef struct gate_wasm_reentry gate_wasm_reentry;

typedef enum gate_wasm_status {
  GATE_WASM_STATUS_OK = 0,
  GATE_WASM_STATUS_ERROR = 1,
  GATE_WASM_STATUS_PANIC = 2,
  GATE_WASM_STATUS_FUEL = 3,
  GATE_WASM_STATUS_DEADLINE = 4,
  GATE_WASM_STATUS_MEMORY = 5,
  GATE_WASM_STATUS_TRANSFER = 6,
} gate_wasm_status;

gate_wasm_slice gate_wasm_runtime_version(void);
gate_wasm_runtime *gate_wasm_runtime_new(
    gate_wasm_slice component, uintptr_t go_host, gate_wasm_limits limits,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_metadata(
    const gate_wasm_runtime *runtime, gate_wasm_plugin_metadata *output,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_init(
    gate_wasm_runtime *runtime, uint64_t context_id, uint64_t proxy_id,
    gate_wasm_owned_sample *output, gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_on_event(
    gate_wasm_runtime *runtime, uint64_t proxy_id, gate_wasm_slice input,
    gate_wasm_owned_bytes *output, gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_allocate(
    gate_wasm_runtime *runtime, uint64_t bytes, uint64_t *output,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_runtime_spin(
    gate_wasm_runtime *runtime, gate_wasm_owned_bytes *error);
int32_t gate_wasm_reentry_on_event(
    gate_wasm_reentry *reentry, uint64_t proxy_id, gate_wasm_slice input,
    gate_wasm_owned_bytes *output, gate_wasm_owned_bytes *error);
void gate_wasm_runtime_free(gate_wasm_runtime *runtime);
void gate_wasm_owned_bytes_free(gate_wasm_owned_bytes value);
void gate_wasm_owned_sample_free(gate_wasm_owned_sample value);

#endif
