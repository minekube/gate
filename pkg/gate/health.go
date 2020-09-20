package gate

// TODO move to gate package for services to register
/*func (g *Gate) runHealthService(stop <-chan struct{}) error {
	probe := p.config.Health
	run, err := health.New(probe.Bind)
	if err != nil {
		return fmt.Errorf("error creating health probe service: %w", err)
	}
	p.log.Info("Health probe service running", "addr", probe.Bind)
	return run(stop, p.healthCheck)
}


// pings the proxy to check health
func (p *Proxy) healthCheck(c context.Context) (*rpc.HealthCheckResponse, error) {
	ctx, cancel := context.WithTimeout(c, time.Second)
	defer cancel()

	var dialer net.Dialer
	client, err := dialer.DialContext(ctx, "tcp", p.config.Bind)
	if err != nil {
		return &rpc.HealthCheckResponse{Status: rpc.HealthCheckResponse_NOT_SERVING}, nil
	}
	defer client.Close()

	return &rpc.HealthCheckResponse{Status: rpc.HealthCheckResponse_SERVING}, nil
}

*/
