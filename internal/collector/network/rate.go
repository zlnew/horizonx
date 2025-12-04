package network

import "time"

func (c *Collector) collectMetric() (NetworkMetric, error) {
	rx, tx, err := readTotals()
	if err != nil {
		return NetworkMetric{}, err
	}

	now := time.Now()
	var rxSpeed float64
	var txSpeed float64

	if !c.lastTime.IsZero() {
		elapsed := now.Sub(c.lastTime).Seconds()
		if elapsed > 0 && rx >= c.lastRxBytes && tx >= c.lastTxBytes {
			rxSpeed = float64(rx-c.lastRxBytes) * 8 / elapsed / 1e6
			txSpeed = float64(tx-c.lastTxBytes) * 8 / elapsed / 1e6
		}
	}

	c.lastRxBytes = rx
	c.lastTxBytes = tx
	c.lastTime = now

	return NetworkMetric{
		RXBytes: rx,
		TXBytes: tx,
		RXSpeed: rxSpeed,
		TXSpeed: txSpeed,
	}, nil
}
