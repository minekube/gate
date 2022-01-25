package deadline

import "time"

type deadline struct {
	timeout      chan struct{} // close-only
	timeoutTimer *time.Timer   // closes timeout
}

func (d *deadline) SetDeadline(t time.Time) error {
	if d.timeoutTimer != nil {
		d.timeoutTimer.Stop()
	}
	d.timeoutTimer = time.AfterFunc(time.Until(t), func() {
		select {
		case <-d.timeout:
		default:
			close(d.timeout)
		}
	})
	return nil
}
