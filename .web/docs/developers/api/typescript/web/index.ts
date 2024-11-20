import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { GateService } from '@buf/minekube_gate.connectrpc_es/minekube/gate/v1/gate_service_connect.js';

const transport = createConnectTransport({
  baseUrl: 'http://localhost:8080',
});

async function main() {
  const client = createClient(GateService, transport);
  const res = await client.listServers({});
  console.log(JSON.stringify(res.servers, null, 2));
}
void main();
