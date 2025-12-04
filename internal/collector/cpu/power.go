package cpu

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"zlnew/monitor-agent/pkg"
)

func (c *Collector) readPowerWatt() float64 {
	var raw float64 = 0

	if watt, err := c.readRAPL(); err == nil && watt > 0 {
		raw = watt
	} else if watt, err := readHwmon(); err == nil && watt > 0 {
		raw = watt
	}

	ema := c.powerEMA

	if raw > 0 {
		ema.Add(raw)
	}

	return ema.Value()
}

func (c *Collector) readRAPL() (float64, error) {
	b, err := os.ReadFile("/sys/class/powercap/intel-rapl/intel-rapl:0/energy_uj")
	if err != nil {
		return 0, err
	}

	energy, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)

	now := time.Now()
	if c.lastEnergy == 0 {
		c.lastEnergy = energy
		c.lastTime = now
		return 0, nil
	}

	deltaEnergy := float64(energy - c.lastEnergy)
	deltaTime := now.Sub(c.lastTime).Seconds()

	if energy < c.lastEnergy {
		deltaEnergy = float64((^uint64(0) - c.lastEnergy) + energy)
	}

	c.lastEnergy = energy
	c.lastTime = now

	watt := (deltaEnergy / 1e6) / deltaTime

	return watt, nil
}

func readHwmon() (float64, error) {
	matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/power*_input")
	targets := []string{
		"zenpower",
		"zenpower3",
		"amd_smu",
		"ryzen_smu",
		"rapl",
		"intel-rapl",
		"intel-rapl-msr",
	}

	for _, f := range matches {
		dir := filepath.Dir(f)
		namePath := filepath.Join(dir, "name")

		nameBytes, err := os.ReadFile(namePath)
		if err != nil {
			continue
		}

		name := strings.TrimSpace(string(nameBytes))
		if !pkg.ContainsAny(name, targets) {
			continue
		}

		b, err := os.ReadFile(f)
		if err == nil {
			v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			return v / 1e6, nil
		}
	}

	return 0, nil
}
