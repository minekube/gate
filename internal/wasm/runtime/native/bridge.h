#ifndef GATE_WASM_BRIDGE_H
#define GATE_WASM_BRIDGE_H

#include "host/include/gate_wasm_native.h"

int32_t gate_wasm_go_context_cancelled(
    uintptr_t host, uint64_t context_id, uint8_t *cancelled,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_go_transform(
    uintptr_t host, uint64_t proxy_id, gate_wasm_sample_view input,
    gate_wasm_owned_sample *output, gate_wasm_owned_bytes *error);
int32_t gate_wasm_go_emit_nested(
    uintptr_t host, gate_wasm_reentry *reentry, uint64_t proxy_id,
    gate_wasm_slice input, gate_wasm_owned_bytes *output,
    gate_wasm_owned_bytes *error);
int32_t gate_wasm_go_invoke(
    uintptr_t host, gate_wasm_callback_reentry *reentry,
    uint32_t operation_id, gate_wasm_slice input,
    gate_wasm_owned_bytes *output, gate_wasm_owned_bytes *error);
int32_t gate_wasm_go_register_callback(
    uintptr_t host, uint32_t callback_type_id, uint64_t guest_id,
    uint64_t *handle, gate_wasm_owned_bytes *error);
int32_t gate_wasm_go_drop_resource(
    uintptr_t host, uint64_t handle, gate_wasm_owned_bytes *error);

int32_t gate_wasm_reentry_call(
    gate_wasm_reentry *reentry, uint64_t proxy_id, gate_wasm_slice input,
    gate_wasm_owned_bytes *output, gate_wasm_owned_bytes *error);
uintptr_t gate_wasm_current_thread_id(void);
void gate_wasm_owned_bytes_set(
    gate_wasm_owned_bytes *value, uint8_t *ptr, size_t len, size_t cap);
void gate_wasm_owned_bytes_list_set(
    gate_wasm_owned_bytes_list *value, gate_wasm_owned_bytes *ptr, size_t len,
    size_t cap);

#endif
